package telegram

import "context"

type Client interface {
	SendMessages(context.Context) error
}
