# orders-svc — Features

- **Cart** — add, update and remove items; cart lives in Redis for 7 days. Entry: `Orders.AddToCart`, `Orders.UpdateCartItem`, `Orders.GetCart`.
- **Checkout** — turns the cart into an order and charges it through payments-svc. Entry: `Orders.Checkout`.
- **Order history** — paginated list of a customer's orders. Entry: `Orders.ListOrders`.
- **Order details** — one order with items and status timeline. Entry: `Orders.GetOrder`.
- **Cancel order** — allowed in `pending` and `paid`; refunds paid orders in full. Entry: `Orders.CancelOrder`.
- **Payment status sync** — marks orders paid or refunded from payment events. Entry: `payment.succeeded` and `payment.refunded` consumers.
- **Order events** — publishes `order.created` and `order.cancelled`. Entry: `internal/events/publisher.go`.
