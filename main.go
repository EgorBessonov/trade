package main

import (
	"context"
	balanceService "github.com/EgorBessonov/balance-service/protocol"
	priceService "github.com/EgorBessonov/price-service/protocol"
	"github.com/EgorBessonov/trade/internal/config"
	"github.com/EgorBessonov/trade/internal/model"
	"github.com/EgorBessonov/trade/internal/repository"
	"github.com/EgorBessonov/trade/internal/server"
	"github.com/EgorBessonov/trade/internal/service"
	tradeService "github.com/EgorBessonov/trade/protocol"
	"github.com/caarlos0/env"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg := config.Config{}
	if err := env.Parse(&cfg); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("trade service: can't parse config")
	}
	balanceConn, err := grpc.Dial(cfg.BalanceServerPort, grpc.WithInsecure())
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("trade service: can't connect to balance service")
	}
	priceConn, err := grpc.Dial(cfg.PriceServerPort, grpc.WithInsecure())
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("trade service: can't connect to price service")
	}
	postgres, err := pgxpool.Connect(context.Background(), cfg.PostgresURL)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("trade service: can't connect to postgres database")
	}
	defer func() {
		if err := balanceConn.Close(); err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Error("trade service: error while closing connection to balance service")
		}
		if err := priceConn.Close(); err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Error("trade service: error while closing connection to price service")
		}
		postgres.Close()
	}()
	exitContext, cancelFunc := context.WithCancel(context.Background())
	exitChan := make(chan os.Signal, 1)
	signal.Notify(exitChan,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGQUIT,
		syscall.SIGTERM)
	balanceCli := balanceService.NewBalanceClient(balanceConn)
	priceCli := priceService.NewPriceClient(priceConn)
	rps := repository.NewPostgresRepository(postgres)
	tService := service.NewService(exitContext, balanceCli, priceCli, rps)
	tServer := server.NewServer(tService)
	go newgRPCConnection(cfg.TradeServerPort, tServer)
	err = tService.NewUser("ec08e0b2-92e2-11ec-9f27-0242ac110002")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("can't create new user")
	}
	time.Sleep(20 * time.Second)
	positionID, err := tService.OpenPosition(context.Background(), &model.OpenRequest{
		UserID:     "ec08e0b2-92e2-11ec-9f27-0242ac110002",
		ShareType:  1,
		ShareCount: 15,
		Price:      10,
	})
	logrus.Printf(positionID)
	<-exitChan
	cancelFunc()
}
func newgRPCConnection(port string, s *server.Server) {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("trade service: can't create gRPC server")
	}
	gServer := grpc.NewServer()
	tradeService.RegisterTraderServer(gServer, s)
	logrus.Printf("traded service: listening at %s", lis.Addr())
	if err = gServer.Serve(lis); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("trade service: gRPC server failed")
	}
}
