// Package tgbot defines the Telegram delivery contract shared by application
// services without coupling them to a concrete bot client.
package tgbot

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

// TelegramSender is the operation required to deliver a Telegram message.
type TelegramSender interface {
	Send(tgbotapi.Chattable) (tgbotapi.Message, error)
}
