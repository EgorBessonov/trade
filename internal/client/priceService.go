package client

import (
	"fmt"
	priceService "github.com/EgorBessonov/price-service/protocol"
	"github.com/EgorBessonov/trade/internal/model"
	"sync"
)

type PriceService struct {
	Client    priceService.PriceClient
	shareList map[int32]*model.Share
	mutex     sync.RWMutex
}

func NewPriceClient(pCli priceService.PriceClient) *PriceService {
	return &PriceService{
		Client:    pCli,
		shareList: make(map[int32]*model.Share),
	}
}

//SaveOrUpdate method set new prices in cache
func (p *PriceService) SaveOrUpdate(share *model.Share) {
	p.mutex.Lock()
	p.shareList[share.ShareType] = share
	p.mutex.Unlock()
}

//Get method returns share prices
func (p *PriceService) Get(shareType int32) (*model.Share, error) {
	p.mutex.RLock()
	if value, ok := p.shareList[shareType]; ok {
		return value, nil
	}
	p.mutex.RUnlock()
	return nil, fmt.Errorf("share list: can't find value")
}

func (p *PriceService) GetBidPrice(shareType int32) (float32, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if value, ok := p.shareList[shareType]; ok {
		return value.Bid, nil
	}
	return 0.0, fmt.Errorf("share list: can't find bid value")
}

func (p *PriceService) GetAskPrice(shareType int32) (float32, error) {
	p.mutex.RLock()
	if value, ok := p.shareList[shareType]; ok {
		return value.Ask, nil
	}
	p.mutex.RUnlock()
	return 0.0, fmt.Errorf("share list: can't find bid value")
}
