package features

import (
	"context"
	"math/rand"
	"sync/atomic"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

var (
	autoTypingEnabled    atomic.Bool
	autoRecordingEnabled atomic.Bool
)

func GetAutoTypingEnabled() bool {
	return autoTypingEnabled.Load()
}

func SetAutoTypingEnabled(val bool) {
	autoTypingEnabled.Store(val)
}

func GetAutoRecordingEnabled() bool {
	return autoRecordingEnabled.Load()
}

func SetAutoRecordingEnabled(val bool) {
	autoRecordingEnabled.Store(val)
}

func HandleAutoPresence(client *whatsmeow.Client, msg *events.Message) {
	chatJID := msg.Info.Chat.String()

	if chatJID == "status@broadcast" {
		return
	}

	botJID := client.Store.ID.User
	if msg.Info.Sender.User == botJID {
		return
	}

	ctx := context.Background()

	if GetAutoTypingEnabled() {
		go sendAutoTyping(ctx, client, msg.Info.Chat)
	}

	if GetAutoRecordingEnabled() {
		go sendAutoRecording(ctx, client, msg.Info.Chat)
	}
}

func sendAutoTyping(ctx context.Context, client *whatsmeow.Client, chat types.JID) {
	delay := time.Duration(500+rand.Intn(1000)) * time.Millisecond
	time.Sleep(delay)

	err := client.SendChatPresence(ctx, chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	if err != nil {
		return
	}

	typingDuration := 15 * time.Second
	time.Sleep(typingDuration)

	client.SendChatPresence(ctx, chat, types.ChatPresencePaused, types.ChatPresenceMediaText)
}

func sendAutoRecording(ctx context.Context, client *whatsmeow.Client, chat types.JID) {
	delay := time.Duration(500+rand.Intn(1000)) * time.Millisecond
	time.Sleep(delay)

	err := client.SendChatPresence(ctx, chat, types.ChatPresenceComposing, types.ChatPresenceMediaAudio)
	if err != nil {
		return
	}

	recordingDuration := 15 * time.Second
	time.Sleep(recordingDuration)

	client.SendChatPresence(ctx, chat, types.ChatPresencePaused, types.ChatPresenceMediaAudio)
}
