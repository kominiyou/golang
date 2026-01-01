package commands

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"whatsapp-bot/core"
)

const (
	ColorGreen = "\033[32m"
)

func HandleMenuCommand(client *whatsmeow.Client, chatJID types.JID, messageID string, senderJID types.JID) {
	ctx := context.Background()

	config := core.GetConfig()

	onlineStatus := "❌ OFF"
	if config.AutoOnline {
		onlineStatus = "✅ ON"
	}

	typingStatus := "❌ OFF"
	if config.AutoTyping {
		typingStatus = "✅ ON"
	}

	recordStatus := "❌ OFF"
	if config.AutoRecording {
		recordStatus = "✅ ON"
	}

	readStoryStatus := "❌ OFF"
	if config.AutoReadStory {
		readStoryStatus = "✅ ON"
	}

	likeStoryStatus := "❌ OFF"
	if config.AutoLikeStory {
		likeStoryStatus = "✅ ON"
	}

	storyDelayStatus := "❌ Normal (1s)"
	if config.StoryRandomDelay {
		storyDelayStatus = "✅ Random (1-20s)"
	}

	menuText := fmt.Sprintf(`╔═══════════════════════
║ 🤖 BOT MENU
╚═══════════════════════

📋 FITUR YANG TERSEDIA:

1️⃣ Auto Online: %s
   .online on/off

2️⃣ Auto Typing: %s
   .typing on/off

3️⃣ Auto Recording: %s
   .record on/off

4️⃣ Auto Read Story: %s
   .readstory on/off

5️⃣ Auto Like Story: %s
   .likestory on/off

6️⃣ Story Delay: %s
   .storydelay on/off

━━━━━━━━━━━━━━━━━━━━

📱 JADIBOT COMMANDS:
• .jadibot 6289xxx - Daftar jadibot
• .listjadibot - Lihat daftar jadibot
• .deljadibot 6289xxx - Hapus jadibot

━━━━━━━━━━━━━━━━━━━━

ℹ️ COMMAND LAINNYA:
• .info - Cek status fitur
• .bot - Cek bot aktif
• .status - Lihat status fitur
• .menu - Lihat menu ini

━━━━━━━━━━━━━━━━━━━━
💡 Gunakan command untuk ubah setting!`,
		onlineStatus, typingStatus, recordStatus, readStoryStatus, likeStoryStatus, storyDelayStatus)

	replyMsg := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(menuText),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:    proto.String(messageID),
				Participant: proto.String(senderJID.String()),
			},
		},
	}

	_, err := client.SendMessage(ctx, chatJID, replyMsg)
	if err != nil {
		fmt.Printf("%s⚠️ Failed to send menu: %v%s\n", ColorYellow, err, ColorReset)
	} else {
		fmt.Printf("%s📋 Menu sent%s\n", ColorGreen, ColorReset)
	}
}
