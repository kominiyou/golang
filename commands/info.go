package commands

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"whatsapp-bot/core"
)

const (
	ColorReset  = "\033[0m"
	ColorCyan   = "\033[36m"
	ColorYellow = "\033[33m"
)

func sendReaction(ctx context.Context, client *whatsmeow.Client, chatJID types.JID, messageID string, emoji string) {
	reactionMsg := &waProto.Message{
		ReactionMessage: &waProto.ReactionMessage{
			Text: proto.String(emoji),
			Key: &waProto.MessageKey{
				RemoteJID: proto.String(chatJID.String()),
				ID:        proto.String(messageID),
			},
		},
	}

	client.SendMessage(ctx, chatJID, reactionMsg)
}

func getRamInfo() (totalMB uint64, usedMB uint64, percentUsed float64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	totalMB = m.TotalAlloc / 1024 / 1024
	usedMB = m.Alloc / 1024 / 1024

	if totalMB > 0 {
		percentUsed = float64(usedMB) / float64(totalMB) * 100
	}

	return
}

func sendBotImage(ctx context.Context, client *whatsmeow.Client, chatJID types.JID, caption string, messageID string, senderJID types.JID) error {
	imageFile := "img/bot.png"
	imageData, err := os.ReadFile(imageFile)
	if err != nil {
		return err
	}

	uploaded, err := client.Upload(ctx, imageData, whatsmeow.MediaImage)
	if err != nil {
		return err
	}

	imgMsg := &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String("image/png"),
			FileLength:    proto.Uint64(uint64(len(imageData))),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			Caption:       proto.String(caption),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:    proto.String(messageID),
				Participant: proto.String(senderJID.String()),
			},
		},
	}

	_, err = client.SendMessage(ctx, chatJID, imgMsg)
	if err != nil {
		return err
	}

	return nil
}

func HandleInfoCommand(client *whatsmeow.Client, chatJID types.JID, messageID string, senderJID types.JID) {
	ctx := context.Background()

	go func() {
		sendReaction(ctx, client, chatJID, messageID, "⏳")
	}()

	time.Sleep(600 * time.Millisecond)

	config := core.GetConfig()

	onlineIcon := "❌"
	onlineStatus := "OFF"
	if config.AutoOnline {
		onlineIcon = "✅"
		onlineStatus = "ON"
	}

	typingIcon := "❌"
	typingStatus := "OFF"
	if config.AutoTyping {
		typingIcon = "✅"
		typingStatus = "ON"
	}

	recordIcon := "❌"
	recordStatus := "OFF"
	if config.AutoRecording {
		recordIcon = "✅"
		recordStatus = "ON"
	}

	readStoryIcon := "❌"
	readStoryStatus := "OFF"
	if config.AutoReadStory {
		readStoryIcon = "✅"
		readStoryStatus = "ON"
	}

	likeStoryIcon := "❌"
	likeStoryStatus := "OFF"
	if config.AutoLikeStory {
		likeStoryIcon = "✅"
		likeStoryStatus = "ON"
	}

	storyDelayIcon := "❌"
	storyDelayStatus := "Normal (1s)"
	if config.StoryRandomDelay {
		storyDelayIcon = "✅"
		storyDelayStatus = "Random (1-20s)"
	}

	now := time.Now()
	currentTime := now.Format("15:04:05")
	currentDate := now.Format("02/01/2006")

	totalRAM, usedRAM, ramPercent := getRamInfo()

	infoText := fmt.Sprintf(
		"┏━━━━━━━━━━━━━━━━━━━━━━\n"+
			"┃ *🤖 BOT STATUS*\n"+
			"┗━━━━━━━━━━━━━━━━━━━━━━\n"+
			"\n"+
			"*> Auto Online*\n"+
			"  Status: `%s` %s\n"+
			"\n"+
			"*> Auto Typing*\n"+
			"  Status: `%s` %s\n"+
			"\n"+
			"*> Auto Recording*\n"+
			"  Status: `%s` %s\n"+
			"\n"+
			"*> Auto Read Story*\n"+
			"  Status: `%s` %s\n"+
			"\n"+
			"*> Auto Like Story*\n"+
			"  Status: `%s` %s\n"+
			"\n"+
			"*> Story Delay*\n"+
			"  Mode: `%s` %s\n"+
			"\n"+
			"*> System RAM*\n"+
			"  Total: `%d MB`\n"+
			"  Used: `%d MB` (%.1f%%)\n"+
			"\n"+
			"┌─────────────────────\n"+
			"│ ⏰ _Waktu:_ `%s`\n"+
			"│ 📅 _Tanggal:_ `%s`\n"+
			"└─────────────────────\n"+
			"\n"+
			"_💡 Use *.menu* for more commands_",
		onlineStatus, onlineIcon,
		typingStatus, typingIcon,
		recordStatus, recordIcon,
		readStoryStatus, readStoryIcon,
		likeStoryStatus, likeStoryIcon,
		storyDelayStatus, storyDelayIcon,
		totalRAM, usedRAM, ramPercent,
		currentTime, currentDate)

	imgErr := sendBotImage(ctx, client, chatJID, infoText, messageID, senderJID)
	if imgErr != nil {
		replyMsg := &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: proto.String(infoText),
				ContextInfo: &waProto.ContextInfo{
					StanzaID:    proto.String(messageID),
					Participant: proto.String(senderJID.String()),
				},
			},
		}

		_, err := client.SendMessage(ctx, chatJID, replyMsg)
		if err != nil {
			go func() {
				sendReaction(ctx, client, chatJID, messageID, "❌")
			}()
		} else {
			go func() {
				time.Sleep(200 * time.Millisecond)
				sendReaction(ctx, client, chatJID, messageID, "✅")
			}()
		}
		return
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		sendReaction(ctx, client, chatJID, messageID, "✅")
	}()

	fmt.Printf("%sℹ️ Info sent%s\n", ColorCyan, ColorReset)
}
