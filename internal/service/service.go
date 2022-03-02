package service

import (
	"context"
	"encoding/json"
	"fmt"
	balanceService "github.com/EgorBessonov/balance-service/protocol"
	priceService "github.com/EgorBessonov/price-service/protocol"
	"github.com/EgorBessonov/trade/internal/client"
	"github.com/EgorBessonov/trade/internal/model"
	"github.com/EgorBessonov/trade/internal/repository"
	"github.com/EgorBessonov/trade/internal/user"
	"github.com/jackc/pgx/v4"
	"github.com/sirupsen/logrus"
	"sync"
)

//Service struct
type Service struct {
	balanceService *client.BalanceService
	priceService   *client.PriceService
	rps            *repository.PostgresRepository
	users          map[string]*user.User
	closeChan      chan model.CloseRequest
	mutex          sync.RWMutex
}

//NewService returns new Service instance and run goroutine which close loss positions
func NewService(ctx context.Context, b balanceService.BalanceClient, p priceService.PriceClient, rps *repository.PostgresRepository) *Service {
	s := Service{
		balanceService: client.NewBalanceClient(b),
		priceService:   client.NewPriceClient(p),
		users:          make(map[string]*user.User),
		closeChan:      make(chan model.CloseRequest),
		rps:            rps,
		mutex:          sync.RWMutex{},
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case closePosition := <-s.closeChan:
				err := s.ClosePosition(context.Background(), &closePosition)
				if err != nil {
					logrus.WithFields(logrus.Fields{
						"error": err,
					}).Error("error while closing loss position")
				}
			}
		}
	}()
	go s.SubscribePriceService(ctx)
	go s.catchPostgresNotify(ctx)
	return &s
}

func (s *Service) TEST(ctx context.Context, request *model.OpenRequest) (string, error) {
	positionID, err := s.open(ctx, request, 15)
	if err != nil {
		return "", err
	}
	return positionID, nil
}

//OpenPosition create new position and update user balance
func (s *Service) OpenPosition(ctx context.Context, request *model.OpenRequest) (string, error) {
	s.mutex.RLock()
	u, ok := s.users[request.UserID]
	s.mutex.RUnlock()
	if !ok {
		return "", fmt.Errorf("service: can't open position - user wasn't found")
	}
	var currentPrice float32
	ok, err := s.priceService.CheckOpenPrice(request.ShareType, request.Price, request.IsSale)
	if err != nil {
		return "", fmt.Errorf("service: can't open position - %e", err)
	}
	if !ok {
		return "", fmt.Errorf("service: can't open position - invalid price")
	}
	requiredBalance := request.Price * float32(request.ShareCount)
	if !u.CheckBalance(requiredBalance) {
		return "", fmt.Errorf("service: can't open position - unsufficient balance")
	}
	positionID, err := s.open(ctx, &model.OpenRequest{
		ShareType:  request.ShareType,
		ShareCount: request.ShareCount,
		Price:      currentPrice,
		IsSale:     false},
		requiredBalance)
	if err != nil {
		return "", err
	}
	u.Withdraw(requiredBalance)
	return positionID, nil
}

//ClosePosition method close position and update user balance
func (s *Service) ClosePosition(ctx context.Context, request *model.CloseRequest) error {
	s.mutex.RLock()
	u, ok := s.users[request.UserID]
	s.mutex.RUnlock()
	if !ok {
		return fmt.Errorf("service: can't close position - user wasn't found")
	}
	position := u.GetPosition(request.ShareType, request.PositionID)
	if position == nil {
		return fmt.Errorf("service: can't close position- can't get position info")
	}
	ok, err := s.priceService.CheckClosePrice(request.ShareType, request.Price, request.IsSale)
	if err != nil {
		return fmt.Errorf("service: can't close position - %e", err)
	}
	if !ok {
		return fmt.Errorf("service: can't close position - invalid price")
	}
	fullPrice := request.Price * float32(position.ShareCount)
	profit := position.PNL(request.Price)
	err = s.close(ctx, &model.CloseRequest{
		PositionID: request.PositionID,
		Price:      request.Price},
		fullPrice, profit)
	if err != nil {
		return fmt.Errorf("service; can't close position")
	}
	u.Withdraw(fullPrice)
	return nil
}

//NewUser method get user balance from database and create new user instance in trade service
func (s *Service) NewUser(userID string) error {
	userBalance, err := s.rps.GetUserBalance(context.Background(), userID)
	if err != nil {
		return fmt.Errorf("service: can't create new user")
	}
	positions, err := s.rps.GetPositionsByID(context.Background(), userID)
	if err != nil {
		return fmt.Errorf("service: can't and user - %e", err)
	}
	u := user.NewUser(context.Background(), positions, userBalance, s.closeChan)
	s.mutex.Lock()
	s.users[userID] = u
	s.mutex.Unlock()
	return nil
}

//SubscribePriceService method get share prices from price service
func (s *Service) SubscribePriceService(ctx context.Context) {
	streamOpts := []int32{1, 2, 3, 4, 5}
	stream, err := s.priceService.Client.Get(ctx, &priceService.GetRequest{Name: streamOpts})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("client: can't send request to price server")
	}
	for {
		select {
		case <-stream.Context().Done():
			return
		default:
			shares, err := stream.Recv()
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"error": err,
				}).Error("client: error while reading gRPC stream")
			}
			share := &model.Share{
				ShareType: shares.Share.Name,
				Bid:       shares.Share.Bid,
				Ask:       shares.Share.Ask,
				UpdatedAt: shares.Share.Time,
			}
			s.priceService.SaveOrUpdate(share)
			s.updatePrice(&model.PriceUpdate{
				ShareType: share.ShareType,
				Ask:       share.Ask,
				Bid:       share.Bid,
			})
		}
	}
}

func (s *Service) updatePrice(updatedPrice *model.PriceUpdate) {
	s.mutex.Lock()
	for _, u := range s.users {
		u.UpdatePrice(updatedPrice)
	}
	s.mutex.Unlock()
}

func (s *Service) open(ctx context.Context, request *model.OpenRequest, balanceShift float32) (string, error) {
	tx, err := s.rps.DBconn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	if request.IsSale {
		positionID, err := s.rps.OpenSalePosition(ctx, tx, request)
		if err != nil {
			txErr := tx.Rollback(ctx)
			if txErr != nil {
				logrus.WithFields(logrus.Fields{
					"error": txErr,
				}).Error("service: can't close transaction")
			}
			return "", err
		}
		_, err = s.balanceService.Withdraw(ctx, request.UserID, balanceShift)
		if err != nil {
			txErr := tx.Rollback(ctx)
			if txErr != nil {
				logrus.WithFields(logrus.Fields{
					"error": txErr,
				}).Error("service: can't close transaction")
			}
			return "", err
		}
		if err := tx.Commit(ctx); err != nil {
			txErr := tx.Rollback(ctx)
			if txErr != nil {
				logrus.WithFields(logrus.Fields{
					"error": txErr,
				}).Error("service: can't close transaction")
			}
			return "", err
		}
		return positionID, nil
	}
	positionID, err := s.rps.OpenBuyPosition(ctx, tx, request)
	if err != nil {
		txErr := tx.Rollback(ctx)
		if txErr != nil {
			logrus.WithFields(logrus.Fields{
				"error": txErr,
			}).Error("service: can't close transaction")
		}
		return "", err
	}
	_, err = s.balanceService.Withdraw(ctx, request.UserID, balanceShift)
	if err != nil {
		txErr := tx.Rollback(ctx)
		if txErr != nil {
			logrus.WithFields(logrus.Fields{
				"error": txErr,
			}).Error("service: can't close transaction")
		}
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		txErr := tx.Rollback(ctx)
		if txErr != nil {
			logrus.WithFields(logrus.Fields{
				"error": txErr,
			}).Error("service: can't close transaction")
		}
		return "", err
	}
	return positionID, nil
}

func (s *Service) close(ctx context.Context, request *model.CloseRequest, balanceShift, profit float32) error {
	tx, err := s.rps.DBconn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		err = tx.Rollback(ctx)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Error("service: can't close transaction")
		}
	}()
	if request.IsSale {
		err = s.rps.CloseSalePosition(ctx, tx, request.Price, profit, request.PositionID)
		if err != nil {
			return err
		}
		_, err = s.balanceService.Refill(ctx, request.UserID, balanceShift)
		if err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	err = s.rps.CloseBuyPosition(ctx, tx, request.Price, profit, request.PositionID)
	if err != nil {
		return err
	}
	_, err = s.balanceService.Refill(ctx, request.UserID, balanceShift)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) catchPostgresNotify(ctx context.Context) {
	postgresNotify, err := s.rps.DBconn.Acquire(context.Background())
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("service: acquiring connection failed")
	}
	defer postgresNotify.Release()
	_, err = postgresNotify.Exec(context.Background(), `listen notify_chat`)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("service: error while listening to chat channel")
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
			notification, err := postgresNotify.Conn().WaitForNotification(context.Background())
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"error": err,
				}).Error("service: error while waiting for postgres notifications")
			}
			position := model.Position{}
			if err := json.Unmarshal([]byte(notification.Payload), &position); err != nil {
				logrus.WithFields(logrus.Fields{
					"error": err,
				}).Error("service: error while parsing postgres notification")
			}
			if position.IsOpened {
				s.mutex.Lock()
				s.users[position.UserID].AddPosition(&position)
				s.mutex.Unlock()
			} else {
				s.mutex.Lock()
				s.users[position.UserID].ClosePosition(&position)
				s.mutex.Unlock()
			}
		}
	}
}
