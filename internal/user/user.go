//Package user represents user logic in trade service
package user

import (
	"context"
	"github.com/EgorBessonov/trade/internal/model"
	"math"
	"sync"
)

//User type represents user behavior in trade service
type User struct {
	id        string
	balance   float32
	priceCh   chan model.PriceUpdate
	closeCh   chan model.CloseRequest
	positions map[int32]map[string]*model.Position
	mutex     sync.RWMutex
}

//NewUser method returns User instance and run goroutine which close user loss positions
func NewUser(ctx context.Context, userBalance float32, closeChan chan model.CloseRequest) *User {
	user := User{
		balance:   userBalance,
		priceCh:   make(chan model.PriceUpdate),
		positions: make(map[int32]map[string]*model.Position),
		closeCh:   closeChan,
		mutex:     sync.RWMutex{},
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case updatedShare := <-user.priceCh:
				user.mutex.RLock()
				for positionID, position := range user.positions[updatedShare.ShareType] {
					if takeProfit(position) || stopLoss(position) {
						if position.IsSale {
							user.closeCh <- model.CloseRequest{
								UserID:     user.id,
								ShareType:  updatedShare.ShareType,
								PositionID: positionID,
								Price:      updatedShare.Bid,
							}
						} else {
							user.closeCh <- model.CloseRequest{
								UserID:     user.id,
								ShareType:  updatedShare.ShareType,
								PositionID: positionID,
								Price:      updatedShare.Ask,
							}
						}
					}
				}
				user.mutex.RUnlock()
				positionID, ok := user.marginCall(&updatedShare)
				if ok {
					user.closeCh <- model.CloseRequest{
						UserID:     user.id,
						ShareType:  updatedShare.ShareType,
						PositionID: positionID,
						Price:      updatedShare.Ask,
					}
				}
			}
		}
	}()
	return &user
}

//ChangeBalance method updates user balance
func (u *User) ChangeBalance(shift float32) {
	u.mutex.Lock()
	u.balance += shift
	u.mutex.Unlock()
}

//AddPosition method add new user position
func (u *User) AddPosition(p *model.Position) {
	u.mutex.Lock()
	if _, ok := u.positions[p.ShareType]; ok {
		u.balance -= p.Ask * float32(p.ShareCount)
		u.positions[p.ShareType][p.PositionID] = p
	}
	u.mutex.Unlock()
}

//ClosePosition method close user position
func (u *User) ClosePosition(p *model.Position, price float32) {
	u.mutex.Lock()
	if _, ok := u.positions[p.ShareType][p.PositionID]; ok {
		u.balance += price * float32(p.ShareCount)
		delete(u.positions[p.ShareType], p.PositionID)
	}
	u.mutex.Unlock()
}

//AddShare create new share positions map if price service get new share type
func (u *User) AddShare(shareType int32) {
	u.mutex.Lock()
	if _, ok := u.positions[shareType]; !ok {
		u.positions[shareType] = make(map[string]*model.Position)
	}
	u.mutex.Unlock()
}

//Withdraw method decrease user balance
func (u *User) Withdraw(shift float32) {
	u.mutex.Lock()
	u.balance -= shift
	u.mutex.Unlock()
}

//Refill method increase user balance
func (u *User) Refill(shift float32) {
	u.mutex.Lock()
	u.balance += shift
	u.mutex.Unlock()
}

//GetPosition method return user position
func (u *User) GetPosition(shareType int32, positionID string) *model.Position {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	if _, ok := u.positions[shareType][positionID]; ok {
		return u.positions[shareType][positionID]
	}
	return nil
}

//GetBalance method returns current user balance
func (u *User) GetBalance() float32 {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	return u.balance
}

//UpdatePrice method send new share price in user chan
func (u *User) UpdatePrice(update *model.PriceUpdate) {
	u.mutex.Lock()
	u.priceCh <- *update
	u.mutex.Unlock()
}

func (u *User) marginCall(up *model.PriceUpdate) (string, bool) {
	var positionID string
	profit := math.MaxFloat32
	u.mutex.RLock()
	currentBalance := u.balance
	for _, position := range u.positions[up.ShareType] {
		pnl := position.PNL(up.Ask)
		if float32(profit) > pnl {
			positionID = position.PositionID
		}
		currentBalance += pnl
	}
	u.mutex.RUnlock()
	return positionID, currentBalance < 0
}

func takeProfit(p *model.Position) bool {
	if p.IsSale {
		return p.Bid <= p.TakeProfit
	}
	return p.Ask >= p.TakeProfit
}

func stopLoss(p *model.Position) bool {
	if p.IsSale {
		return p.Bid >= p.StopLoss
	}
	return p.Ask <= p.StopLoss
}

//CheckBalance return false if user balance less than required balance for opening position
func (u *User) CheckBalance(requiredPrice float32) bool {
	return u.balance > requiredPrice
}
