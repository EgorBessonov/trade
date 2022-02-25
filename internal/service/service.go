package service

import (
	"context"
	"fmt"
	balanceService "github.com/EgorBessonov/balance-service/protocol"
	priceService "github.com/EgorBessonov/price-service/protocol"
	"github.com/EgorBessonov/trade/internal/client"
	"github.com/EgorBessonov/trade/internal/model"
	"github.com/EgorBessonov/trade/internal/repository"
	"github.com/EgorBessonov/trade/internal/user"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
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
	return &s
}

//OpenPosition create new position and update user balance
func (s *Service) OpenPosition(ctx context.Context, request *model.OpenRequest) (string, error) {
	s.mutex.RLock()
	user, ok := s.users[request.UserID]
	s.mutex.RUnlock()
	if !ok {
		return "", fmt.Errorf("service: can't open position - user didn't find")
	}
	if request.IsSale {
		askPrice, err := s.priceService.GetAskPrice(request.ShareType)
		if err != nil {
			return "", fmt.Errorf("service: can't open position - %e", err)
		}
		/*if request.Price != askPrice {
			return "", fmt.Errorf("service: can't open position - invalid price")
		}*/
		positionID, err := s.rps.OpenSalePosition(ctx, &model.OpenRequest{
			UserID:     "",
			ShareType:  request.ShareType,
			ShareCount: request.ShareCount,
			Ask:        askPrice,
			IsSale:     false,
		})
		if err != nil {
			return "", fmt.Errorf("service: can't open position - %e", err)
		}
		fullPrice := askPrice * float32(request.ShareCount)
		_, err = s.balanceService.Withdraw(ctx, request.UserID, fullPrice)
		if err != nil {
			return "", fmt.Errorf("service: can't top up balance - %e", err)
		}
		user.Refill(fullPrice)
		user.AddPosition(&model.Position{
			PositionID: positionID,
			ShareType:  request.ShareType,
			ShareCount: request.ShareCount,
			Ask:        askPrice,
			OpenTime:   time.Now().Format(time.RFC3339Nano),
			IsSale:     true,
		})
		return positionID, nil
	}
	bidPrice, err := s.priceService.GetBidPrice(request.ShareType)
	if err != nil {
		return "", fmt.Errorf("service: can't open position - %e", err)
	}
	/*if request.Price != bidPrice {
		return "", fmt.Errorf("service: can't open position - invalid price")
	}*/
	requiredBalance := request.Bid * float32(request.ShareCount)
	if !user.CheckBalance(requiredBalance) {
		return "", fmt.Errorf("service: can't open position - unsufficient balance")
	}
	positionID, err := s.rps.OpenBuyPosition(ctx, &model.OpenRequest{
		ShareType:  request.ShareType,
		ShareCount: request.ShareCount,
		Bid:        bidPrice,
	})
	if err != nil {
		return "", fmt.Errorf("service: can't open position - %e", err)
	}
	_, err = s.balanceService.Withdraw(ctx, request.UserID, requiredBalance)
	if err != nil {
		return "", fmt.Errorf("service: can't top up balance - %e", err)
	}
	user.Withdraw(requiredBalance)
	user.AddPosition(&model.Position{
		PositionID: positionID,
		ShareType:  request.ShareType,
		ShareCount: request.ShareCount,
		Bid:        request.Bid,
		OpenTime:   time.Now().Format(time.RFC3339Nano),
		IsSale:     false,
	})
	return positionID, nil
}

//ClosePosition method close position and update user balance
func (s *Service) ClosePosition(ctx context.Context, request *model.CloseRequest) error {
	s.mutex.RLock()
	user, ok := s.users[request.UserID]
	s.mutex.RUnlock()
	if !ok {
		return fmt.Errorf("service: can't close position - user didn't find")
	}
	position := user.GetPosition(request.ShareType, request.PositionID)
	if position == nil {
		return fmt.Errorf("service: can't close position- can't get position info")
	}
	if position.IsSale {
		bid, err := s.priceService.GetAskPrice(request.ShareType)
		if err != nil {
			return fmt.Errorf("service: can't close position - %e", err)
		}
		if request.Price != bid {
			return fmt.Errorf("service: can't close position - invalid price")
		}
		requiredBalance := float32(position.ShareCount) * bid
		if !user.CheckBalance(requiredBalance) {
			return fmt.Errorf("service: can't close position - unsufficient balance")
		}
		profit := (position.Ask - bid) * float32(position.ShareCount)
		err = s.rps.CloseSalePosition(ctx, request.Price, profit, request.PositionID)
		if err != nil {
			return fmt.Errorf("service: can't close position - %e", err)
		}
		_, err = s.balanceService.Withdraw(ctx, request.UserID, requiredBalance)
		if err != nil {
			return fmt.Errorf("service: can't refill balance - %e", err)
		}
		user.ClosePosition(&model.Position{
			PositionID: request.PositionID,
			ShareType:  request.ShareType,
			ShareCount: position.ShareCount},
			-requiredBalance)
		return nil
	}
	ask, err := s.priceService.GetAskPrice(request.ShareType)
	if err != nil {
		return fmt.Errorf("service: can't close position - %e", err)
	}
	if request.Price != ask {
		return fmt.Errorf("service: can't close position - invalid price")
	}
	fullPrice := request.Price * float32(position.ShareCount)
	profit := (request.Price - position.Bid) * float32(position.ShareCount)
	err = s.rps.CloseBuyPosition(ctx, request.Price, profit, request.PositionID)
	if err != nil {
		return fmt.Errorf("service: can't close position - %e", err)
	}
	_, err = s.balanceService.Refill(ctx, request.UserID, fullPrice)
	if err != nil {
		return fmt.Errorf("service: can't refill balance - %e", err)
	}
	user.ClosePosition(&model.Position{
		PositionID: request.PositionID,
		ShareType:  request.ShareType,
		ShareCount: position.ShareCount},
		fullPrice)
	return nil
}

//NewUser method get user balance from database and create new user instance in trade service
func (s *Service) NewUser(userID string) error {
	userBalance, err := s.rps.GetUserBalance(context.Background(), userID)
	if err != nil {
		return fmt.Errorf("service: can't create new user")
	}
	u := user.NewUser(context.Background(), userBalance, s.closeChan)
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
			s.addShare(share.ShareType)
			s.updatePrice(&model.PriceUpdate{
				ShareType:    share.ShareType,
				UpdatedPrice: share.Ask,
			})
		}
	}
}

func (s *Service) addShare(shareType int32) {
	s.mutex.Lock()
	for _, user := range s.users {
		user.AddShare(shareType)
	}
	s.mutex.Unlock()
}

func (s *Service) updatePrice(updatedPrice *model.PriceUpdate) {
	s.mutex.Lock()
	for _, user := range s.users {
		user.UpdatePrice(updatedPrice)
	}
	s.mutex.Unlock()
}
