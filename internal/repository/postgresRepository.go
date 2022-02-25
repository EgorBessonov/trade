package repository

import (
	"context"
	"fmt"
	"github.com/EgorBessonov/trade/internal/model"
	"github.com/jackc/pgx/v4/pgxpool"
	"time"
)

type PostgresRepository struct {
	DBconn *pgxpool.Pool
}

func NewPostgresRepository(conn *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{DBconn: conn}
}

//OpenPosition method create position record in database
func (rps *PostgresRepository) OpenPosition(ctx context.Context, openRequest *model.OpenRequest) (string, error) {
	var positionID string
	err := rps.DBconn.QueryRow(ctx, `insert into positions (sharetype, sharecount, bid, opentime)
	values ($1, $2, $3, $4) returning positionid`, openRequest.ShareType, openRequest.ShareCount, openRequest.Price, time.Now().Format(time.RFC3339Nano)).Scan(&positionID)
	if err != nil {
		return "", fmt.Errorf("repository: can't open position - %e", err)
	}

	return positionID, nil
}

//ClosePosition method close position record in database
func (rps *PostgresRepository) ClosePosition(ctx context.Context, closePrice, profit float32, positionID string) error {
	_, err := rps.DBconn.Exec(ctx, `update positions set ask=$1, closeTime=$2, profit=$3 where positionID=$4`, closePrice, time.Now().Format(time.RFC3339Nano), profit, positionID)
	if err != nil {
		return fmt.Errorf("repository: can't close position - %e", err)
	}
	return nil
}

//GetPosition method returns position record from database
func (rps *PostgresRepository) GetPosition(ctx context.Context, positionID string) (*model.Position, error) {
	var position model.Position
	err := rps.DBconn.QueryRow(ctx, `select * from positions where positionid = $1`, positionID).Scan(&position.PositionID, &position.ShareType, &position.ShareCount, &position.Bid, &position.Ask, &position.OpenTime, &position.CloseTime, &position.Profit, &position.IsOpened)
	if err != nil {
		return nil, fmt.Errorf("repository: can't get position - %e", err)
	}
	return &position, nil
}

//GetUserBalance method returns user balance from repository
func (rps *PostgresRepository) GetUserBalance(ctx context.Context, userID string) (float32, error) {
	var userBalance float32
	err := rps.DBconn.QueryRow(ctx, "select balance from users where id=$1", userID).Scan(&userBalance)
	if err != nil {
		return 0, fmt.Errorf("repository: can't get user balance - %e", err)
	}
	return userBalance, nil
}
