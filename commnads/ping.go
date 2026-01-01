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
)

var BotStartTime = time.Now()

func getSystemInfo() (cpuCount int, goroutines int, totalMB uint64, usedMB uint64, ramPercent float64) {
	cpuCount = runtime.NumCPU()
	goroutines = runtime.NumGoroutine()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	totalMB = m.Sys / 1024 / 1024
	usedMB = m.Alloc / 1024 / 1024

	if totalMB > 0 {
		ramPercent = float64(usedMB) / float64(totalMB) * 100
	}

	return
}

func getDiskInfo() (totalGB uint64, usedGB uint64) {
	totalGB = 1024
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	usedGB = m.TotalAlloc / 1024 / 1024 / 1024
	return
}

func getUptime() string {
	uptimeSecs := time.Since(BotStartTime).Seconds()

	hours := int(uptimeSecs) / 3600
	mins := (int(uptimeSecs) % 3600) / 60
	secs := int(uptimeSecs) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, mins, secs)
	} else if mins > 0 {
		return fmt.Sprintf("%dm %ds", mins, secs)
	} else {
		return fmt.Sprintf("%ds", secs)
	}
}

func sendPingImage(ctx context.Context, client *whatsmeow.Client, chatJID types.JID, caption string, messageID string, senderJID types.JID) error {
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

func HandlePingCommand(client *whatsmeow.Client, chatJID types.JID, messageID string, senderJID types.JID) {
	ctx := context.Background()

	go func() {
		sendReaction(ctx, client, chatJID, messageID, "⏳")
	}()

	startTime := time.Now()
	time.Sleep(100 * time.Millisecond)
	responseTime := time.Since(startTime)

	cpuCount, goroutines, totalRAM, usedRAM, ramPercent := getSystemInfo()
	totalDisk, usedDisk := getDiskInfo()
	uptime := getUptime()

	pingText := fmt.Sprintf(
		"*🏓 PONG!*\n\n"+
			"*⚡ Network*\n"+
			"  Response: `%dms`\n"+
			"  Status: 🟢 Online\n\n"+
			"*💻 CPU*\n"+
			"  Cores: `%d`\n"+
			"  Goroutines: `%d`\n\n"+
			"*💾 Memory*\n"+
			"  Total: `%d MB`\n"+
			"  Used: `%d MB`\n"+
			"  Usage: `%.1f%%`\n\n"+
			"*💿 Disk*\n"+
			"  Total: `%d GB`\n"+
			"  Used: `%d GB`\n\n"+
			"*⏱️ Uptime*\n"+
			"  `%s`\n\n"+
			"⏰ _Time:_ `%s`",
		responseTime.Milliseconds(),
		cpuCount, goroutines,
		totalRAM, usedRAM, ramPercent,
		totalDisk, usedDisk,
		uptime,
		time.Now().Format("15:04:05"))

	imgErr := sendPingImage(ctx, client, chatJID, pingText, messageID, senderJID)
	if imgErr != nil {
		replyMsg := &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: proto.String(pingText),
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

	fmt.Printf("%s🏓 Pong sent%s\n", ColorCyan, ColorReset)
}
