package utils

import (
	"context"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

var (
	lidMappingCache = make(map[string]string)
	lidMappingMutex sync.RWMutex
)

type SenderInfo struct {
	ID   string
	LID  string
	Name string
}

func IsLIDUser(jid string) bool {
	return strings.HasSuffix(jid, "@lid")
}

func IsPNUser(jid string) bool {
	return strings.HasSuffix(jid, "@s.whatsapp.net")
}

func IsGroupJID(jid string) bool {
	return strings.HasSuffix(jid, "@g.us")
}

func IsStatusBroadcast(jid string) bool {
	return strings.HasPrefix(jid, "status@broadcast") || strings.Contains(jid, "broadcast")
}

func StoreLIDMapping(lid, phoneNumber string) {
	if lid == "" || phoneNumber == "" {
		return
	}
	lidMappingMutex.Lock()
	defer lidMappingMutex.Unlock()
	lidMappingCache[lid] = phoneNumber
	lidMappingCache[phoneNumber] = lid
}

func GetPNForLID(lid string) string {
	lidMappingMutex.RLock()
	defer lidMappingMutex.RUnlock()
	if pn, exists := lidMappingCache[lid]; exists {
		return pn
	}
	return ""
}

func GetLIDForPN(phoneNumber string) string {
	lidMappingMutex.RLock()
	defer lidMappingMutex.RUnlock()
	if lid, exists := lidMappingCache[phoneNumber]; exists && IsLIDUser(lid) {
		return lid
	}
	return ""
}

func ResolveLIDToPN(client *whatsmeow.Client, senderJID types.JID, chatJID types.JID) SenderInfo {
	info := SenderInfo{
		ID:   senderJID.String(),
		LID:  "",
		Name: "",
	}

	ctx := context.Background()
	jidStr := senderJID.String()

	if IsLIDUser(jidStr) {
		info.LID = jidStr

		cachedPN := GetPNForLID(jidStr)
		if cachedPN != "" && IsPNUser(cachedPN) {
			info.ID = cachedPN
		} else {
			resolvedPN := resolveLIDFromStore(client, senderJID)
			if resolvedPN != "" {
				info.ID = resolvedPN
				StoreLIDMapping(jidStr, resolvedPN)
			} else if IsGroupJID(chatJID.String()) {
				resolvedPN = resolveLIDFromGroup(client, senderJID, chatJID)
				if resolvedPN != "" {
					info.ID = resolvedPN
					StoreLIDMapping(jidStr, resolvedPN)
				}
			}
		}
	} else if IsPNUser(jidStr) {
		info.ID = jidStr
		cachedLID := GetLIDForPN(jidStr)
		if cachedLID != "" {
			info.LID = cachedLID
		}
	}

	if parsedJID, err := types.ParseJID(info.ID); err == nil {
		contact, err := client.Store.Contacts.GetContact(ctx, parsedJID)
		if err == nil {
			if contact.FullName != "" {
				info.Name = contact.FullName
			} else if contact.PushName != "" {
				info.Name = contact.PushName
			}
		}
	}

	if info.Name == "" && senderJID.User != "" {
		info.Name = senderJID.User
	}

	return info
}

func resolveLIDFromStore(client *whatsmeow.Client, lidJID types.JID) string {
	ctx := context.Background()

	if client.Store.LIDs != nil {
		phoneJID, err := client.Store.LIDs.GetPNForLID(ctx, lidJID)
		if err == nil && !phoneJID.IsEmpty() && phoneJID.Server == types.DefaultUserServer {
			return phoneJID.String()
		}
	}

	return ""
}

func resolveLIDFromGroup(client *whatsmeow.Client, lidJID types.JID, groupJID types.JID) string {
	ctx := context.Background()

	groupInfo, err := client.GetGroupInfo(ctx, groupJID)
	if err != nil {
		return ""
	}

	for _, participant := range groupInfo.Participants {
		var phoneNumber string

		if !participant.PhoneNumber.IsEmpty() && participant.PhoneNumber.Server == types.DefaultUserServer {
			phoneNumber = participant.PhoneNumber.String()
		} else if !participant.JID.IsEmpty() && participant.JID.Server == types.DefaultUserServer {
			phoneNumber = participant.JID.String()
		}

		if participant.LID.String() == lidJID.String() {
			if phoneNumber != "" {
				StoreLIDMapping(participant.LID.String(), phoneNumber)
				return phoneNumber
			}
		}

		if participant.JID.String() == lidJID.String() && participant.JID.Server == types.HiddenUserServer {
			if phoneNumber != "" {
				StoreLIDMapping(participant.JID.String(), phoneNumber)
				return phoneNumber
			}
		}

		if !participant.LID.IsEmpty() && phoneNumber != "" {
			StoreLIDMapping(participant.LID.String(), phoneNumber)
		}
	}

	return ""
}

func CacheGroupParticipantMappings(client *whatsmeow.Client, groupJID types.JID) {
	ctx := context.Background()

	groupInfo, err := client.GetGroupInfo(ctx, groupJID)
	if err != nil {
		return
	}

	for _, participant := range groupInfo.Participants {
		var phoneNumber string

		if !participant.PhoneNumber.IsEmpty() && participant.PhoneNumber.Server == types.DefaultUserServer {
			phoneNumber = participant.PhoneNumber.String()
		} else if !participant.JID.IsEmpty() && participant.JID.Server == types.DefaultUserServer {
			phoneNumber = participant.JID.String()
		}

		if !participant.LID.IsEmpty() && phoneNumber != "" {
			lidStr := participant.LID.String()
			if IsLIDUser(lidStr) && IsPNUser(phoneNumber) {
				StoreLIDMapping(lidStr, phoneNumber)
			}
		}

		if participant.JID.Server == types.HiddenUserServer && phoneNumber != "" {
			jidStr := participant.JID.String()
			if IsLIDUser(jidStr) && IsPNUser(phoneNumber) {
				StoreLIDMapping(jidStr, phoneNumber)
			}
		}
	}
}

func CacheAllJoinedGroupsMappings(client *whatsmeow.Client) {
	ctx := context.Background()

	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		return
	}

	for _, group := range groups {
		CacheGroupParticipantMappings(client, group.JID)
	}
}

func ResolveSenderForSelfMode(client *whatsmeow.Client, evt interface{}, isFromMe bool) SenderInfo {
	botJID := client.Store.ID

	info := SenderInfo{
		ID:   botJID.String(),
		LID:  "",
		Name: "Self",
	}

	if isFromMe {
		info.ID = botJID.User + "@s.whatsapp.net"
		return info
	}

	return info
}

func IsSelfMessage(client *whatsmeow.Client, senderJID types.JID) bool {
	if client.Store.ID == nil {
		return false
	}

	botJID := client.Store.ID

	if senderJID.User == botJID.User {
		return true
	}

	senderStr := senderJID.String()
	botStr := botJID.String()

	if IsLIDUser(senderStr) {
		resolvedPN := GetPNForLID(senderStr)
		if resolvedPN != "" {
			parsedPN, err := types.ParseJID(resolvedPN)
			if err == nil && parsedPN.User == botJID.User {
				return true
			}
		}
	}

	if IsLIDUser(botStr) {
		resolvedPN := GetPNForLID(botStr)
		if resolvedPN != "" && senderStr == resolvedPN {
			return true
		}
	}

	return false
}

func NormalizeJID(jidStr string) string {
	if jidStr == "" {
		return ""
	}

	if IsLIDUser(jidStr) {
		pn := GetPNForLID(jidStr)
		if pn != "" && IsPNUser(pn) {
			return pn
		}
	}

	return jidStr
}

func ExtractPhoneFromJID(jid types.JID) string {
	if jid.User == "" {
		return ""
	}

	parts := strings.Split(jid.User, ":")
	return parts[0]
}

func GetAccurateSenderInfo(client *whatsmeow.Client, senderJID types.JID, chatJID types.JID, isFromMe bool) SenderInfo {
	info := SenderInfo{
		ID:   "",
		LID:  "",
		Name: "",
	}

	if isFromMe {
		botJID := client.Store.ID
		info.ID = botJID.User + "@s.whatsapp.net"
		info.Name = "Self"
		return info
	}

	senderStr := senderJID.String()

	if IsLIDUser(senderStr) {
		info.LID = senderStr

		cachedPN := GetPNForLID(senderStr)
		if cachedPN != "" && IsPNUser(cachedPN) {
			info.ID = cachedPN
		} else {
			if IsGroupJID(chatJID.String()) {
				resolvedPN := resolveLIDFromGroup(client, senderJID, chatJID)
				if resolvedPN != "" {
					info.ID = resolvedPN
					StoreLIDMapping(senderStr, resolvedPN)
				} else {
					info.ID = senderStr
				}
			} else {
				info.ID = senderStr
			}
		}
	} else {
		info.ID = senderStr
		cachedLID := GetLIDForPN(senderStr)
		if cachedLID != "" {
			info.LID = cachedLID
		}
	}

	ctx := context.Background()
	if IsPNUser(info.ID) {
		if parsedJID, err := types.ParseJID(info.ID); err == nil {
			contact, err := client.Store.Contacts.GetContact(ctx, parsedJID)
			if err == nil {
				if contact.FullName != "" {
					info.Name = contact.FullName
				} else if contact.PushName != "" {
					info.Name = contact.PushName
				}
			}
		}
	}

	if info.Name == "" {
		info.Name = ExtractPhoneFromJID(senderJID)
	}

	return info
}
