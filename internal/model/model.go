//Package model represents models witch are used in trade service
package model

//Share type represents Share structure in trade service
type Share struct {
	ShareType int32
	Bid       float32
	Ask       float32
	UpdatedAt string
}

//OpenRequest type represents OpenPosition request structure in trade service
type OpenRequest struct {
	UserID     string
	ShareType  int32
	ShareCount int32
	Price      float32
}

//PriceUpdate type represents info which is needed for share price updating
type PriceUpdate struct {
	ShareType    int32
	UpdatedPrice float32
}

//CloseRequest type represents ClosePosition request structure in trade service
type CloseRequest struct {
	UserID     string
	ShareType  int32
	PositionID string
	Price      float32
}

//Position type represents position structure in trade service
type Position struct {
	PositionID string
	ShareType  int32
	ShareCount int32
	Bid        float32
	Ask        float32
	OpenTime   string
	CloseTime  string
	Profit     float32
	IsOpened   bool
}

//PNL method calculates pnl for position
func (p *Position) PNL(ask float32) float32 {
	return p.Ask - ask*float32(p.ShareCount)
}
