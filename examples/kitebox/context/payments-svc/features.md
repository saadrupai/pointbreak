# payments-svc — Features

- **Charge** — charges a saved card or a payment token for an order. Entry: `Payments.Charge`.
- **Refund** — full or partial refund of a charge, up to the remaining amount. Entry: `Payments.Refund`.
- **Payment methods** — save, list and remove cards (Stripe customer + payment method ids only). Entry: `Payments.SavePaymentMethod`, `ListPaymentMethods`, `RemovePaymentMethod`.
- **Stripe webhooks** — confirms async charge and refund results and publishes events. Entry: `POST /webhooks/stripe`.
- **Payment events** — `payment.succeeded`, `payment.failed`, `payment.refunded`. Entry: `internal/adapters/nats/publisher.go`.
