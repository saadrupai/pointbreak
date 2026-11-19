# notifications-svc — Features

- **Order confirmation email** — sent when an order is created. Entry: `order.created` consumer, template `email/order_created`.
- **Payment receipt** — email after a successful charge. Entry: `payment.succeeded` consumer, template `email/payment_receipt`.
- **Cancellation notice** — email + SMS when an order is cancelled. Entry: `order.cancelled` consumer.
- **Refund notice** — email when a refund succeeds. Entry: `payment.refunded` consumer, template `email/refund`.
- **Retries** — failed sends retry 5 times with backoff. Entry: asynq `notify:send`.
- **Admin resend** — resend any past message. Entry: `Notifications.Resend`.
