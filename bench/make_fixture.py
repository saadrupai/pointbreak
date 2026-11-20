"""Writes the benchmark fixture: a small, realistic Kitebox codebase (3 Go services + a contracts repo)
that matches examples/kitebox/context. Read-only tasks are asked against it. Run: python3 make_fixture.py <out-dir>"""
import os, sys, textwrap
out = sys.argv[1]
F = {}
def f(path, body): F[path] = textwrap.dedent(body).lstrip()

# ---------------- kitebox-proto ----------------
f("kitebox-proto/README.md", """
# kitebox-proto
Shared gRPC and event contracts. Generated Go code is published as github.com/kitebox/proto-go.
""")
f("kitebox-proto/events/v1/order.proto", """
syntax = "proto3";
package events.v1;
// Published by orders-svc on NATS subjects order.created and order.cancelled.
message OrderCreated { string order_public_id = 1; int64 user_id = 2; int64 total_amount = 3; string currency = 4; }
message OrderCancelled { string order_public_id = 1; int64 user_id = 2; string reason = 3; bool refund_requested = 4; }
""")
f("kitebox-proto/events/v1/payment.proto", """
syntax = "proto3";
package events.v1;
// Published by payments-svc on NATS subjects payment.succeeded, payment.failed, payment.refunded.
message PaymentSucceeded { string charge_public_id = 1; string order_public_id = 2; int64 amount = 3; string currency = 4; }
message PaymentFailed { string charge_public_id = 1; string order_public_id = 2; string failure_code = 3; }
message PaymentRefunded {
  string refund_public_id = 1;
  string charge_public_id = 2;
  string order_public_id = 3;
  int64 amount = 4;
  string currency = 5;
  int64 user_id = 6;
}
""")
f("kitebox-proto/payments/v1/payments.proto", """
syntax = "proto3";
package payments.v1;
service Payments {
  rpc Charge(ChargeRequest) returns (ChargeResponse);
  rpc Refund(RefundRequest) returns (RefundResponse);
  rpc SavePaymentMethod(SavePaymentMethodRequest) returns (PaymentMethod);
  rpc ListPaymentMethods(ListPaymentMethodsRequest) returns (ListPaymentMethodsResponse);
}
message ChargeRequest { string order_public_id = 1; int64 amount = 2; string currency = 3; string payment_method_id = 4; }
message ChargeResponse { string charge_public_id = 1; string status = 2; }
message RefundRequest { string charge_public_id = 1; int64 amount = 2; string reason = 3; }
message RefundResponse { string refund_public_id = 1; string status = 2; }
message SavePaymentMethodRequest { int64 user_id = 1; string token = 2; }
message PaymentMethod { string id = 1; string brand = 2; string last4 = 3; }
message ListPaymentMethodsRequest { int64 user_id = 1; }
message ListPaymentMethodsResponse { repeated PaymentMethod methods = 1; }
""")

# ---------------- orders-svc ----------------
f("orders-svc/go.mod", "module github.com/kitebox/orders-svc\n\ngo 1.23\n")
f("orders-svc/README.md", """
# orders-svc
Orders and their life cycle. gRPC on :13001. Commands: serve, consume, migrate.
""")
f("orders-svc/config.example.yml", """
grpc: { port: 13001 }
postgres: { dsn: "postgres://user:pass@localhost:5432/orders?sslmode=disable" }
redis: { url: "redis://localhost:6379/0" }
nats: { url: "nats://localhost:4222" }
payments: { addr: "payments-svc:13002" }
""")
f("orders-svc/cmd/orders/main.go", """
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: orders serve|consume|migrate")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		serve()
	case "consume":
		consume()
	case "migrate":
		migrate(os.Args[2:])
	}
}
""")
f("orders-svc/cmd/orders/serve.go", """
package main

import (
	"github.com/kitebox/orders-svc/internal/clients"
	"github.com/kitebox/orders-svc/internal/config"
	grpcdelivery "github.com/kitebox/orders-svc/internal/delivery/grpc"
	"github.com/kitebox/orders-svc/internal/events"
	"github.com/kitebox/orders-svc/internal/repository/postgres"
	"github.com/kitebox/orders-svc/internal/usecase"
)

// serve wires config -> Postgres -> Redis -> payments client -> NATS -> gRPC server.
func serve() {
	cfg := config.MustLoad()
	db := postgres.MustOpen(cfg.Postgres.DSN)
	repo := postgres.NewOrderRepository(db)
	payments := clients.NewPaymentsClient(cfg.Payments.Addr)
	pub := events.NewPublisher(cfg.NATS.URL)
	uc := usecase.NewOrders(repo, payments, pub)
	grpcdelivery.NewServer(uc).ListenAndServe(cfg.GRPC.Port)
}

// consume runs the NATS consumers for payment events.
func consume() {
	cfg := config.MustLoad()
	db := postgres.MustOpen(cfg.Postgres.DSN)
	uc := usecase.NewOrders(postgres.NewOrderRepository(db), nil, events.NewPublisher(cfg.NATS.URL))
	events.NewConsumer(cfg.NATS.URL, uc).Run()
}
""")
f("orders-svc/internal/domain/order.go", """
package domain

import (
	"context"
	"errors"
	"time"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusPaid      Status = "paid"
	StatusShipped   Status = "shipped"
	StatusDelivered Status = "delivered"
	StatusCancelled Status = "cancelled"
	StatusRefunded  Status = "refunded"
)

var (
	ErrNotFound          = errors.New("order not found")
	ErrCannotCancel      = errors.New("order cannot be cancelled in its current status")
	ErrInvalidTransition = errors.New("invalid status transition")
)

type Order struct {
	ID          int64
	PublicID    string
	UserID      int64
	Status      Status
	TotalAmount int64 // minor units
	Currency    string
	PaymentID   string // payments-svc charges.public_id
	Items       []OrderItem
	CreatedAt   time.Time
}

type OrderItem struct {
	SKU       string
	Name      string
	UnitPrice int64
	Quantity  int
}

type OrderRepository interface {
	Create(ctx context.Context, o *Order) error
	GetByPublicID(ctx context.Context, publicID string) (*Order, error)
	UpdateStatus(ctx context.Context, id int64, from, to Status, reason string) error
	SetPaymentID(ctx context.Context, id int64, paymentID string) error
	ListByUser(ctx context.Context, userID int64, limit int, cursor string) ([]Order, string, error)
}

type PaymentsClient interface {
	Charge(ctx context.Context, orderPublicID string, amount int64, currency, paymentMethodID string) (chargeID string, err error)
	Refund(ctx context.Context, chargeID string, amount int64, reason string) (refundID string, err error)
}

type Publisher interface {
	OrderCreated(ctx context.Context, o *Order) error
	OrderCancelled(ctx context.Context, o *Order, reason string, refundRequested bool) error
}
""")
f("orders-svc/internal/usecase/orders.go", """
package usecase

import "github.com/kitebox/orders-svc/internal/domain"

type Orders struct {
	repo     domain.OrderRepository
	payments domain.PaymentsClient
	pub      domain.Publisher
}

func NewOrders(r domain.OrderRepository, p domain.PaymentsClient, pub domain.Publisher) *Orders {
	return &Orders{repo: r, payments: p, pub: pub}
}
""")
f("orders-svc/internal/usecase/checkout.go", """
package usecase

import (
	"context"
	"fmt"

	"github.com/kitebox/orders-svc/internal/domain"
)

// Checkout creates the order from the cart and charges it through payments-svc.
func (u *Orders) Checkout(ctx context.Context, o *domain.Order, paymentMethodID string) error {
	o.Status = domain.StatusPending
	if err := u.repo.Create(ctx, o); err != nil {
		return fmt.Errorf("create order: %w", err)
	}
	chargeID, err := u.payments.Charge(ctx, o.PublicID, o.TotalAmount, o.Currency, paymentMethodID)
	if err != nil {
		return fmt.Errorf("charge order %s: %w", o.PublicID, err)
	}
	if err := u.repo.SetPaymentID(ctx, o.ID, chargeID); err != nil {
		return fmt.Errorf("save payment id: %w", err)
	}
	return u.pub.OrderCreated(ctx, o)
}
""")
f("orders-svc/internal/usecase/cancel.go", """
package usecase

import (
	"context"
	"fmt"

	"github.com/kitebox/orders-svc/internal/domain"
)

// CancelOrder cancels a pending or paid order. Paid orders are refunded in full through payments-svc;
// the order moves to refunded later, when the payment.refunded event arrives.
func (u *Orders) CancelOrder(ctx context.Context, publicID, reason string) error {
	o, err := u.repo.GetByPublicID(ctx, publicID)
	if err != nil {
		return err
	}
	if o.Status != domain.StatusPending && o.Status != domain.StatusPaid {
		return domain.ErrCannotCancel
	}
	refund := o.Status == domain.StatusPaid
	if refund {
		if _, err := u.payments.Refund(ctx, o.PaymentID, o.TotalAmount, reason); err != nil {
			return fmt.Errorf("refund order %s: %w", publicID, err)
		}
	}
	if err := u.repo.UpdateStatus(ctx, o.ID, o.Status, domain.StatusCancelled, reason); err != nil {
		return fmt.Errorf("cancel order %s: %w", publicID, err)
	}
	return u.pub.OrderCancelled(ctx, o, reason, refund)
}
""")
f("orders-svc/internal/usecase/status.go", """
package usecase

import (
	"context"

	"github.com/kitebox/orders-svc/internal/domain"
)

// MarkPaid handles payment.succeeded.
func (u *Orders) MarkPaid(ctx context.Context, orderPublicID string) error {
	o, err := u.repo.GetByPublicID(ctx, orderPublicID)
	if err != nil {
		return err
	}
	if o.Status == domain.StatusPaid {
		return nil // redelivery
	}
	return u.repo.UpdateStatus(ctx, o.ID, domain.StatusPending, domain.StatusPaid, "payment_succeeded")
}

// MarkRefunded handles payment.refunded for a cancelled order.
func (u *Orders) MarkRefunded(ctx context.Context, orderPublicID string) error {
	o, err := u.repo.GetByPublicID(ctx, orderPublicID)
	if err != nil {
		return err
	}
	if o.Status == domain.StatusRefunded {
		return nil
	}
	if o.Status != domain.StatusCancelled {
		return domain.ErrInvalidTransition
	}
	return u.repo.UpdateStatus(ctx, o.ID, domain.StatusCancelled, domain.StatusRefunded, "payment_refunded")
}
""")
f("orders-svc/internal/clients/payments.go", """
package clients

import (
	"context"

	paymentsv1 "github.com/kitebox/proto-go/payments/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

// PaymentsClient calls payments-svc over gRPC. Every call carries an idempotency-key.
type PaymentsClient struct{ c paymentsv1.PaymentsClient }

func NewPaymentsClient(addr string) *PaymentsClient { return &PaymentsClient{c: dial(addr)} }

func (p *PaymentsClient) Charge(ctx context.Context, orderID string, amount int64, currency, pm string) (string, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "idempotency-key", "charge:"+orderID)
	res, err := p.c.Charge(ctx, &paymentsv1.ChargeRequest{OrderPublicId: orderID, Amount: amount, Currency: currency, PaymentMethodId: pm})
	if err != nil {
		return "", err
	}
	return res.ChargePublicId, nil
}

func (p *PaymentsClient) Refund(ctx context.Context, chargeID string, amount int64, reason string) (string, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "idempotency-key", "refund:"+chargeID+":"+uuid.NewString())
	res, err := p.c.Refund(ctx, &paymentsv1.RefundRequest{ChargePublicId: chargeID, Amount: amount, Reason: reason})
	if err != nil {
		return "", err
	}
	return res.RefundPublicId, nil
}
""")
f("orders-svc/internal/events/publisher.go", """
package events

import (
	"context"

	eventsv1 "github.com/kitebox/proto-go/events/v1"
	"github.com/kitebox/orders-svc/internal/domain"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
)

const (
	SubjectOrderCreated   = "order.created"
	SubjectOrderCancelled = "order.cancelled"
)

type Publisher struct{ js jetstream.JetStream }

func NewPublisher(url string) *Publisher { return &Publisher{js: connect(url)} }

func (p *Publisher) OrderCreated(ctx context.Context, o *domain.Order) error {
	b, _ := proto.Marshal(&eventsv1.OrderCreated{OrderPublicId: o.PublicID, UserId: o.UserID, TotalAmount: o.TotalAmount, Currency: o.Currency})
	_, err := p.js.Publish(ctx, SubjectOrderCreated, b)
	return err
}

func (p *Publisher) OrderCancelled(ctx context.Context, o *domain.Order, reason string, refund bool) error {
	b, _ := proto.Marshal(&eventsv1.OrderCancelled{OrderPublicId: o.PublicID, UserId: o.UserID, Reason: reason, RefundRequested: refund})
	_, err := p.js.Publish(ctx, SubjectOrderCancelled, b)
	return err
}
""")
f("orders-svc/internal/events/consumer.go", """
package events

import (
	"context"

	eventsv1 "github.com/kitebox/proto-go/events/v1"
	"github.com/kitebox/orders-svc/internal/usecase"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
)

// Consumer keeps order status in sync with payment events (durable consumer "orders").
type Consumer struct {
	js jetstream.JetStream
	uc *usecase.Orders
}

func NewConsumer(url string, uc *usecase.Orders) *Consumer { return &Consumer{js: connect(url), uc: uc} }

func (c *Consumer) Run() {
	c.subscribe("payment.succeeded", func(ctx context.Context, data []byte) error {
		var e eventsv1.PaymentSucceeded
		if err := proto.Unmarshal(data, &e); err != nil {
			return err
		}
		return c.uc.MarkPaid(ctx, e.OrderPublicId)
	})
	c.subscribe("payment.refunded", func(ctx context.Context, data []byte) error {
		var e eventsv1.PaymentRefunded
		if err := proto.Unmarshal(data, &e); err != nil {
			return err
		}
		return c.uc.MarkRefunded(ctx, e.OrderPublicId)
	})
}
""")
f("orders-svc/internal/repository/postgres/orders.go", """
package postgres

import (
	"context"

	"github.com/kitebox/orders-svc/internal/domain"
	"github.com/kitebox/orders-svc/internal/repository/postgres/db"
)

// OrderRepository uses sqlc-generated queries from queries/orders.sql.
type OrderRepository struct{ q *db.Queries }

func NewOrderRepository(conn db.DBTX) *OrderRepository { return &OrderRepository{q: db.New(conn)} }

func (r *OrderRepository) UpdateStatus(ctx context.Context, id int64, from, to domain.Status, reason string) error {
	// one transaction: status change + history row in order_status_events
	return r.q.UpdateOrderStatusWithEvent(ctx, db.UpdateOrderStatusWithEventParams{ID: id, FromStatus: string(from), ToStatus: string(to), Reason: reason})
}
""")
f("orders-svc/internal/repository/postgres/queries/orders.sql", """
-- name: GetOrderByPublicID :one
SELECT id, public_id, user_id, status, total_amount, currency, payment_id, created_at FROM orders WHERE public_id = $1;

-- name: UpdateOrderStatusWithEvent :exec
WITH upd AS (UPDATE orders SET status = @to_status, updated_at = now() WHERE id = @id AND status = @from_status RETURNING id)
INSERT INTO order_status_events (order_id, from_status, to_status, reason) SELECT id, @from_status, @to_status, @reason FROM upd;

-- name: ListOrdersByUser :many
SELECT * FROM orders WHERE user_id = $1 AND created_at < $2 ORDER BY created_at DESC LIMIT $3;
""")
f("orders-svc/internal/delivery/grpc/handler.go", """
package grpc

import (
	"context"
	"errors"

	ordersv1 "github.com/kitebox/proto-go/orders/v1"
	"github.com/kitebox/orders-svc/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) CancelOrder(ctx context.Context, req *ordersv1.CancelOrderRequest) (*ordersv1.CancelOrderResponse, error) {
	if err := s.uc.CancelOrder(ctx, req.OrderPublicId, req.Reason); err != nil {
		if errors.Is(err, domain.ErrCannotCancel) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		return nil, status.Error(codes.Internal, "cancel failed")
	}
	return &ordersv1.CancelOrderResponse{}, nil
}
""")
f("orders-svc/migrations/000001_init.up.sql", """
CREATE TYPE order_status AS ENUM ('pending', 'paid', 'shipped', 'delivered', 'cancelled', 'refunded');
CREATE TABLE orders (
  id BIGSERIAL PRIMARY KEY,
  public_id TEXT NOT NULL UNIQUE,
  user_id BIGINT NOT NULL,
  status order_status NOT NULL DEFAULT 'pending',
  total_amount BIGINT NOT NULL,
  currency CHAR(3) NOT NULL DEFAULT 'USD',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_orders_user_created ON orders (user_id, created_at DESC);
CREATE TABLE order_items (
  id BIGSERIAL PRIMARY KEY,
  order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  sku TEXT NOT NULL,
  name TEXT NOT NULL,
  unit_price BIGINT NOT NULL,
  quantity INT NOT NULL CHECK (quantity > 0)
);
CREATE INDEX idx_order_items_order ON order_items (order_id);
""")
f("orders-svc/migrations/000001_init.down.sql", "DROP TABLE order_items; DROP TABLE orders; DROP TYPE order_status;\n")
f("orders-svc/migrations/000002_add_payment_id.up.sql", "ALTER TABLE orders ADD COLUMN payment_id TEXT;\n")
f("orders-svc/migrations/000002_add_payment_id.down.sql", "ALTER TABLE orders DROP COLUMN payment_id;\n")
f("orders-svc/migrations/000003_order_status_events.up.sql", """
CREATE TABLE order_status_events (
  id BIGSERIAL PRIMARY KEY,
  order_id BIGINT NOT NULL REFERENCES orders(id),
  from_status order_status,
  to_status order_status NOT NULL,
  reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
""")
f("orders-svc/migrations/000003_order_status_events.down.sql", "DROP TABLE order_status_events;\n")

# ---------------- payments-svc ----------------
f("payments-svc/go.mod", "module github.com/kitebox/payments-svc\n\ngo 1.23\n")
f("payments-svc/README.md", """
# payments-svc
Charges, refunds and saved cards via Stripe. gRPC :13002 (internal only), HTTP :8082 for Stripe webhooks.
""")
f("payments-svc/main.go", "package main\n\nimport \"github.com/kitebox/payments-svc/cmd\"\n\nfunc main() { cmd.Execute() }\n")
f("payments-svc/cmd/serve.go", """
package cmd

import (
	grpcadapter "github.com/kitebox/payments-svc/internal/adapters/grpc"
	natsadapter "github.com/kitebox/payments-svc/internal/adapters/nats"
	"github.com/kitebox/payments-svc/internal/adapters/postgres"
	"github.com/kitebox/payments-svc/internal/adapters/stripe"
	"github.com/kitebox/payments-svc/internal/core"
)

// serve wires config -> Postgres -> Stripe -> NATS -> gRPC (with idempotency interceptor).
func serve() {
	cfg := mustLoad()
	repo := postgres.New(cfg.Postgres.DSN)
	svc := core.NewService(repo, stripe.New(cfg.Stripe.APIKey), natsadapter.NewPublisher(cfg.NATS.URL))
	grpcadapter.NewServer(svc).ListenAndServe(cfg.GRPC.Port)
}
""")
f("payments-svc/cmd/webhooks.go", """
package cmd

import httpadapter "github.com/kitebox/payments-svc/internal/adapters/http"

// webhooks runs POST /webhooks/stripe on :8082.
func webhooks() {
	cfg := mustLoad()
	httpadapter.NewWebhookServer(cfg.Stripe.WebhookSecret, buildService(cfg)).ListenAndServe(cfg.HTTP.Port)
}
""")
f("payments-svc/internal/core/types.go", """
package core

import (
	"context"
	"errors"
)

var (
	ErrRefundExceedsCharge = errors.New("refund amount exceeds the remaining charge amount")
	ErrChargeNotSucceeded  = errors.New("only succeeded charges can be refunded")
)

type Charge struct {
	ID, Amount, RefundedAmount int64
	PublicID, OrderPublicID     string
	Currency, Status            string
	StripePaymentIntentID       string
}

type Refund struct {
	ID, ChargeID, Amount int64
	PublicID, Status     string
	Reason               string
	StripeRefundID       string
}

type Repository interface {
	CreateCharge(ctx context.Context, c *Charge, idempotencyKey string) error
	GetChargeByPublicID(ctx context.Context, publicID string) (*Charge, error)
	CreateRefund(ctx context.Context, r *Refund, idempotencyKey string) error
	MarkRefundSucceeded(ctx context.Context, stripeRefundID string) (*Refund, *Charge, error)
}

type Processor interface {
	CreatePaymentIntent(ctx context.Context, amount int64, currency, paymentMethod, idempotencyKey string) (string, error)
	CreateRefund(ctx context.Context, paymentIntentID string, amount int64, idempotencyKey string) (string, error)
}

type Publisher interface {
	PaymentSucceeded(ctx context.Context, c *Charge) error
	PaymentRefunded(ctx context.Context, r *Refund, c *Charge) error
}
""")
f("payments-svc/internal/core/refund.go", """
package core

import (
	"context"
	"fmt"
)

// Refund refunds part or all of a succeeded charge. The amount can't exceed what is left
// (amount - refunded_amount); the database enforces the same rule with a check constraint.
func (s *Service) Refund(ctx context.Context, chargePublicID string, amount int64, reason, idempotencyKey string) (*Refund, error) {
	c, err := s.repo.GetChargeByPublicID(ctx, chargePublicID)
	if err != nil {
		return nil, fmt.Errorf("get charge %s: %w", chargePublicID, err)
	}
	if c.Status != "succeeded" {
		return nil, ErrChargeNotSucceeded
	}
	if amount > c.Amount-c.RefundedAmount {
		return nil, ErrRefundExceedsCharge
	}
	stripeID, err := s.processor.CreateRefund(ctx, c.StripePaymentIntentID, amount, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("stripe refund: %w", err)
	}
	r := &Refund{ChargeID: c.ID, Amount: amount, Status: "pending", Reason: reason, StripeRefundID: stripeID}
	if err := s.repo.CreateRefund(ctx, r, idempotencyKey); err != nil {
		return nil, fmt.Errorf("save refund: %w", err)
	}
	return r, nil // succeeded later via the Stripe webhook
}

// OnRefundSucceeded is called by the Stripe webhook. It updates refunds.status and
// charges.refunded_amount in one transaction, then publishes payment.refunded.
func (s *Service) OnRefundSucceeded(ctx context.Context, stripeRefundID string) error {
	r, c, err := s.repo.MarkRefundSucceeded(ctx, stripeRefundID)
	if err != nil {
		return err
	}
	return s.pub.PaymentRefunded(ctx, r, c)
}
""")
f("payments-svc/internal/core/charge.go", """
package core

import (
	"context"
	"fmt"
)

type Service struct {
	repo      Repository
	processor Processor
	pub       Publisher
}

func NewService(r Repository, p Processor, pub Publisher) *Service { return &Service{repo: r, processor: p, pub: pub} }

func (s *Service) Charge(ctx context.Context, orderPublicID string, amount int64, currency, pm, idempotencyKey string) (*Charge, error) {
	pi, err := s.processor.CreatePaymentIntent(ctx, amount, currency, pm, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("create payment intent: %w", err)
	}
	c := &Charge{OrderPublicID: orderPublicID, Amount: amount, Currency: currency, Status: "pending", StripePaymentIntentID: pi}
	if err := s.repo.CreateCharge(ctx, c, idempotencyKey); err != nil {
		return nil, err
	}
	return c, nil
}
""")
f("payments-svc/internal/adapters/nats/publisher.go", """
package nats

import (
	"context"

	eventsv1 "github.com/kitebox/proto-go/events/v1"
	"github.com/kitebox/payments-svc/internal/core"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
)

type Publisher struct{ js jetstream.JetStream }

func NewPublisher(url string) *Publisher { return &Publisher{js: connect(url)} }

func (p *Publisher) PaymentSucceeded(ctx context.Context, c *core.Charge) error {
	b, _ := proto.Marshal(&eventsv1.PaymentSucceeded{ChargePublicId: c.PublicID, OrderPublicId: c.OrderPublicID, Amount: c.Amount, Currency: c.Currency})
	_, err := p.js.Publish(ctx, "payment.succeeded", b)
	return err
}

func (p *Publisher) PaymentRefunded(ctx context.Context, r *core.Refund, c *core.Charge) error {
	b, _ := proto.Marshal(&eventsv1.PaymentRefunded{
		RefundPublicId: r.PublicID, ChargePublicId: c.PublicID, OrderPublicId: c.OrderPublicID,
		Amount: r.Amount, Currency: c.Currency,
	})
	_, err := p.js.Publish(ctx, "payment.refunded", b)
	return err
}
""")
f("payments-svc/internal/adapters/http/webhook.go", """
package http

import (
	"net/http"

	"github.com/stripe/stripe-go/v81/webhook"
)

// POST /webhooks/stripe: verifies the signature, then confirms charges and refunds.
func (s *WebhookServer) handle(w http.ResponseWriter, r *http.Request) {
	event, err := webhook.ConstructEvent(readBody(r), r.Header.Get("Stripe-Signature"), s.secret)
	if err != nil {
		http.Error(w, "bad signature", http.StatusBadRequest)
		return
	}
	switch event.Type {
	case "payment_intent.succeeded":
		s.svc.OnChargeSucceeded(r.Context(), event.Data.Object["id"].(string))
	case "charge.refund.updated", "refund.updated":
		s.svc.OnRefundSucceeded(r.Context(), event.Data.Object["id"].(string))
	}
	w.WriteHeader(http.StatusOK)
}
""")
f("payments-svc/internal/adapters/postgres/models.go", """
package postgres

import "time"

type chargeModel struct {
	ID                    int64
	PublicID              string
	OrderPublicID         string
	Amount                int64
	RefundedAmount        int64
	Currency              string
	Status                string
	StripePaymentIntentID string
	IdempotencyKey        string
	CreatedAt, UpdatedAt  time.Time
}

func (chargeModel) TableName() string { return "charges" }

type refundModel struct {
	ID             int64
	PublicID       string
	ChargeID       int64
	Amount         int64
	Status         string
	Reason         string
	StripeRefundID string
	IdempotencyKey string
	CreatedAt      time.Time
}

func (refundModel) TableName() string { return "refunds" }

type paymentMethodModel struct {
	ID                    int64
	UserID                int64
	StripeCustomerID      string
	StripePaymentMethodID string
	Brand, Last4          string
}

func (paymentMethodModel) TableName() string { return "payment_methods" }
""")
f("payments-svc/db/migrations/20261003000000_init.sql", """
-- +goose Up
CREATE TYPE charge_status AS ENUM ('pending', 'succeeded', 'failed');
CREATE TYPE refund_status AS ENUM ('pending', 'succeeded', 'failed');
CREATE TABLE charges (
  id BIGSERIAL PRIMARY KEY,
  public_id TEXT NOT NULL UNIQUE,
  order_public_id TEXT NOT NULL,
  amount BIGINT NOT NULL,
  refunded_amount BIGINT NOT NULL DEFAULT 0,
  currency CHAR(3) NOT NULL,
  status charge_status NOT NULL DEFAULT 'pending',
  stripe_payment_intent_id TEXT,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT refunded_le_amount CHECK (refunded_amount <= amount)
);
CREATE INDEX idx_charges_order ON charges (order_public_id);
CREATE TABLE refunds (
  id BIGSERIAL PRIMARY KEY,
  public_id TEXT NOT NULL UNIQUE,
  charge_id BIGINT NOT NULL REFERENCES charges(id),
  amount BIGINT NOT NULL,
  status refund_status NOT NULL DEFAULT 'pending',
  reason TEXT,
  stripe_refund_id TEXT,
  idempotency_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refunds_charge ON refunds (charge_id);
-- +goose Down
DROP TABLE refunds; DROP TABLE charges; DROP TYPE refund_status; DROP TYPE charge_status;
""")
f("payments-svc/db/migrations/20261021000000_payment_methods.sql", """
-- +goose Up
CREATE TABLE payment_methods (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  stripe_customer_id TEXT NOT NULL,
  stripe_payment_method_id TEXT NOT NULL UNIQUE,
  brand TEXT NOT NULL,
  last4 CHAR(4) NOT NULL,
  deleted_at TIMESTAMPTZ
);
-- +goose Down
DROP TABLE payment_methods;
""")

# ---------------- notifications-svc ----------------
f("notifications-svc/go.mod", "module github.com/kitebox/notifications-svc\n\ngo 1.23\n")
f("notifications-svc/README.md", """
# notifications-svc
Email (SES) and SMS (Twilio) for customers. Consumes order and payment events; retries via asynq.
""")
f("notifications-svc/cmd/notifications/main.go", """
package main

import (
	"os"

	"github.com/kitebox/notifications-svc/internal/consumers"
	"github.com/kitebox/notifications-svc/internal/worker"
)

func main() {
	switch os.Args[1] {
	case "consume":
		consumers.Run(mustLoad())
	case "worker":
		worker.Run(mustLoad())
	}
}
""")
f("notifications-svc/internal/consumers/router.go", """
package consumers

// Run subscribes durable consumer "notifications" to every customer-facing event.
func Run(cfg Config) {
	r := newRouter(cfg)
	r.handle("order.created", orderCreated)
	r.handle("order.cancelled", orderCancelled)
	r.handle("payment.succeeded", paymentSucceeded)
	r.handle("payment.refunded", paymentRefunded)
	r.run()
}
""")
f("notifications-svc/internal/consumers/order_cancelled.go", """
package consumers

import (
	"context"

	eventsv1 "github.com/kitebox/proto-go/events/v1"
	"google.golang.org/protobuf/proto"
)

// orderCancelled sends an email and an SMS. Dedupe keys are per channel.
func orderCancelled(ctx context.Context, d deps, msgID string, data []byte) error {
	var e eventsv1.OrderCancelled
	if err := proto.Unmarshal(data, &e); err != nil {
		return err
	}
	if err := d.enqueue(ctx, "order.cancelled:"+msgID+":email", "email", "email/order_cancelled", e.UserId, &e); err != nil {
		return err
	}
	return d.enqueue(ctx, "order.cancelled:"+msgID+":sms", "sms", "sms/order_cancelled", e.UserId, &e)
}
""")
f("notifications-svc/internal/consumers/payment_refunded.go", """
package consumers

import (
	"context"

	eventsv1 "github.com/kitebox/proto-go/events/v1"
	"google.golang.org/protobuf/proto"
)

// paymentRefunded emails the customer that a refund went through (template email/refund).
func paymentRefunded(ctx context.Context, d deps, msgID string, data []byte) error {
	var e eventsv1.PaymentRefunded
	if err := proto.Unmarshal(data, &e); err != nil {
		return err
	}
	return d.enqueue(ctx, "payment.refunded:"+msgID+":email", "email", "email/refund", e.UserId, &e)
}
""")
f("notifications-svc/internal/consumers/order_created.go", """
package consumers

import (
	"context"

	eventsv1 "github.com/kitebox/proto-go/events/v1"
	"google.golang.org/protobuf/proto"
)

func orderCreated(ctx context.Context, d deps, msgID string, data []byte) error {
	var e eventsv1.OrderCreated
	if err := proto.Unmarshal(data, &e); err != nil {
		return err
	}
	return d.enqueue(ctx, "order.created:"+msgID+":email", "email", "email/order_created", e.UserId, &e)
}
""")
f("notifications-svc/internal/consumers/payment_succeeded.go", """
package consumers

import (
	"context"

	eventsv1 "github.com/kitebox/proto-go/events/v1"
	"google.golang.org/protobuf/proto"
)

func paymentSucceeded(ctx context.Context, d deps, msgID string, data []byte) error {
	var e eventsv1.PaymentSucceeded
	if err := proto.Unmarshal(data, &e); err != nil {
		return err
	}
	return d.enqueue(ctx, "payment.succeeded:"+msgID+":email", "email", "email/payment_receipt", 0, &e)
}
""")
f("notifications-svc/internal/templates/email/refund.tmpl", "Subject: Your refund of {{.Amount | money .Currency}} is on its way\n\nHi, we refunded {{.Amount | money .Currency}} for order {{.OrderPublicId}}.\n")
f("notifications-svc/internal/templates/email/order_cancelled.tmpl", "Subject: Order {{.OrderPublicId}} cancelled\n\nYour order was cancelled. {{if .RefundRequested}}A refund is on its way.{{end}}\n")
f("notifications-svc/internal/templates/sms/order_cancelled.tmpl", "Kitebox: order {{.OrderPublicId}} was cancelled.{{if .RefundRequested}} Refund on its way.{{end}}\n")
f("notifications-svc/internal/worker/send.go", """
package worker

// notify:send task: renders the template and sends via SES (email) or Twilio (sms).
// 5 retries with exponential backoff, then delivery_attempts.status = 'failed'.
const TaskSend = "notify:send"
""")
f("notifications-svc/migrations/000001_init.up.sql", """
CREATE TABLE delivery_attempts (
  id BIGSERIAL PRIMARY KEY,
  dedupe_key TEXT NOT NULL UNIQUE,
  channel TEXT NOT NULL,
  template TEXT NOT NULL,
  recipient TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'queued',
  attempts INT NOT NULL DEFAULT 0,
  last_error TEXT,
  provider_message_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  sent_at TIMESTAMPTZ
);
CREATE INDEX idx_attempts_status_created ON delivery_attempts (status, created_at);
""")

for path, body in F.items():
    p = os.path.join(out, path)
    os.makedirs(os.path.dirname(p), exist_ok=True)
    with open(p, "w") as fh:
        fh.write(body)
print(f"wrote {len(F)} files to {out}")
