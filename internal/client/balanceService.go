package client

import (
	"context"
	"fmt"
	balanceService "github.com/EgorBessonov/balance-service/protocol"
)

type BalanceService struct {
	client balanceService.BalanceClient
}

func NewBalanceClient(bCli balanceService.BalanceClient) *BalanceService {
	return &BalanceService{client: bCli}
}

func (b *BalanceService) CheckBalance(ctx context.Context, userID string, requiredBalance float32) (bool, error) {
	result, err := b.client.Check(ctx, &balanceService.CheckRequest{
		UserId:          userID,
		RequiredBalance: requiredBalance,
	})
	if err != nil {
		return false, fmt.Errorf("client: can't check balance - %e", err)
	}
	return result.Ok, nil
}

func (b *BalanceService) Withdraw(ctx context.Context, userID string, shift float32) (string, error) {
	result, err := b.client.Withdraw(ctx, &balanceService.WithdrawRequest{
		UserId: userID,
		Shift:  shift,
	})
	if err != nil {
		return "", fmt.Errorf("client: can't withdraw balance - %e", err)
	}
	return result.Result, nil
}

func (b *BalanceService) Refill(ctx context.Context, userID string, shift float32) (string, error) {
	result, err := b.client.TopUp(ctx, &balanceService.TopUpRequest{
		UserId: userID,
		Shift:  shift,
	})
	if err != nil {
		return "", fmt.Errorf("client: can't top up balance")
	}
	return result.Result, nil
}
