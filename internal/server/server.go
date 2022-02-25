//Package server represents trade gRPC server handlers
package server

import (
	"context"
	"github.com/EgorBessonov/trade/internal/model"
	"github.com/EgorBessonov/trade/internal/service"
	tradeService "github.com/EgorBessonov/trade/protocol"
	"github.com/sirupsen/logrus"
)

//Server type represents server structure in trade service
type Server struct {
	Service *service.Service
	tradeService.UnimplementedTraderServer
}

//NewServer returns new server instance
func NewServer(service *service.Service) *Server {
	return &Server{
		Service: service,
	}
}

//OpenPosition method open position record
func (s *Server) OpenPosition(ctx context.Context, request *tradeService.OpenPositionRequest) (*tradeService.OpenPositionResponse, error) {
	if request.IsSale {
		positionID, err := s.Service.OpenPosition(ctx, &model.OpenRequest{
			UserID:     request.UserId,
			ShareType:  request.ShareType,
			ShareCount: request.Count,
			Ask:        request.Price,
		})
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Error("server: can't open position")
			return nil, err
		}
		return &tradeService.OpenPositionResponse{PositionID: positionID}, nil
	}
	positionID, err := s.Service.OpenPosition(ctx, &model.OpenRequest{
		UserID:     request.UserId,
		ShareType:  request.ShareType,
		ShareCount: request.Count,
		Bid:        request.Price,
	})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("server: can't open position")
		return nil, err
	}
	return &tradeService.OpenPositionResponse{PositionID: positionID}, nil
}

//ClosePosition method close position record
func (s *Server) ClosePosition(ctx context.Context, request *tradeService.ClosePositionRequest) (*tradeService.ClosePositionResponse, error) {

	err := s.Service.ClosePosition(ctx, &model.CloseRequest{
		PositionID: request.PositionId,
		UserID:     request.UserId,
		Price:      request.Price,
	})
	if err != nil {
		return nil, err
	}
	return &tradeService.ClosePositionResponse{Result: "success"}, nil
}
