package main

import (
	"aladin-golang-playground/internal/aladinbank"
	"aladin-golang-playground/internal/database"
	"context"
	"database/sql"
	"github.com/joho/godotenv"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	_ "github.com/lib/pq"
)

const (
	port = ":50051"
)

type aladinbankServer struct {
	aladinbank.UnimplementedAladinBankServiceServer
	db *sql.DB
}

func (s *aladinbankServer) TransferFunds(ctx context.Context, req *aladinbank.TransferRequest) (*aladinbank.TransferResponse, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Lock rows for update
	_, err = tx.ExecContext(ctx, "SELECT balance FROM accounts WHERE id = $1 FOR UPDATE", req.FromAccountId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to lock from account: %v", err)
	}
	_, err = tx.ExecContext(ctx, "SELECT balance FROM accounts WHERE id = $1 FOR UPDATE", req.ToAccountId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to lock to account: %v", err)
	}

	// Check balance
	var fromBalance float64
	err = tx.QueryRowContext(ctx, "SELECT balance FROM accounts WHERE id = $1", req.FromAccountId).Scan(&fromBalance)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "from account not found: %v", err)
	}
	if fromBalance < req.Amount {
		return nil, status.Errorf(codes.FailedPrecondition, "insufficient balance")
	}

	// Update balances
	_, err = tx.ExecContext(ctx, "UPDATE accounts SET balance = balance - $1 WHERE id = $2", req.Amount, req.FromAccountId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update from account: %v", err)
	}
	_, err = tx.ExecContext(ctx, "UPDATE accounts SET balance = balance + $1 WHERE id = $2", req.Amount, req.ToAccountId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update to account: %v", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &aladinbank.TransferResponse{Success: true}, nil
}

func (s *aladinbankServer) GetBalance(ctx context.Context, req *aladinbank.GetBalanceRequest) (*aladinbank.GetBalanceResponse, error) {
	var balance float64
	err := s.db.QueryRowContext(ctx, "SELECT balance FROM accounts WHERE id = $1", req.AccountId).Scan(&balance)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found: %v", err)
	}
	return &aladinbank.GetBalanceResponse{Balance: balance}, nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("failed to load .env file: %v", err)
	}

	db, err := database.NewDB()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	aladinbank.RegisterAladinBankServiceServer(s, &aladinbankServer{db: db})
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
