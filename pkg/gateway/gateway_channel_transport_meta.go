package gateway

import "strings"

func channelSessionTransportMeta(meta map[string]string) map[string]string {
	transportMeta := map[string]string{}
	for _, key := range []string{"channel_id", "chat_id", "guild_id", "attachment_count"} {
		if v := strings.TrimSpace(meta[key]); v != "" {
			transportMeta[key] = v
		}
	}
	return transportMeta
}
