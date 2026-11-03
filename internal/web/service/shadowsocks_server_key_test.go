package service

import (
	"encoding/base64"
	"fmt"
)

func ss2022Settings(method, serverKey string) string {
	clientKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
	if method != "2022-blake3-aes-128-gcm" {
		clientKey = base64.StdEncoding.EncodeToString(make([]byte, 32))
	}
	return fmt.Sprintf(`{"method":%q,"password":%q,"network":"tcp,udp","clients":[{"email":"ss-key","password":%q,"enable":true}]}`,
		method, serverKey, clientKey)
}
