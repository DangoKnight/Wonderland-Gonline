package login

import "strconv"

// disconnectReasons are the texts the action-0 handler (0x2de0b0) stores
// for each subcommand before the server closes the connection. Subcommand
// 39 and codes past the table use the default.
var disconnectReasons = map[byte]string{
	1: "Too Many Packets", 2: "Fail Q&A 3 Times", 3: "Wrong Pwd", 4: "Disconnected",
	5: "Illegal Activity #1", 6: "Illegal Activity #2", 7: "Event Error", 8: "Incorrect Trigger",
	9: "Wrong Event Table", 10: "Table Error", 11: "Limiter Tiggered", 12: "Illegal Press",
	13: "Stream Violation", 14: "Movement Too Fast", 15: "Delete Successful", 16: "Blocked IP Detected",
	17: "Update Game Files", 18: "Data Altered", 19: "Repeated Login", 20: "Abnormal D/c",
	21: "Abnormal Safe File Data", 22: "Invalid Packet Data", 23: "Name Changed", 24: "Password Too Short",
	25: "Duplicated Name", 26: "Event Trigger Error", 27: "Login Error D/c", 28: "Firewall D/c",
	29: "Too Much Data", 30: "Account Lock", 31: "Login ID Unavailable", 32: "Battle Error",
	33: "Scene Error", 34: "Other Server Login", 35: "Confirm Licence", 36: "Invalid ID",
	37: "Scene Changed", 38: "Wrong Target Scene", 40: "Packet Manipulation", 41: "Relogin & Top Up",
	42: "Battle Error", 43: "Battle Error #0", 44: "Wrong Tigger Scene", 45: "Account Banned",
	46: "Q&A Cheater Detected", 47: "Battle End Error", 48: "Use illegal Skill", 49: "Under 18 y/o Curfew",
	50: "Log into PVP Server", 51: "Can't Login PVP Server", 52: "Can't Login PVP Server", 53: "Server Not Open Yet",
	54: "Left PVP Event", 55: "No Transport On World Map", 56: "Password Too Long", 57: "Pwd & Del Pwd match",
	58: "Beta Access Error", 59: "Relay Server D/c", 60: "Connection lost", 61: "Server Is Busy",
	62: "Incorrect Password", 63: "Mini-Game Error D/c", 64: "Illegal App Used", 65: "Wrong Version",
	66: "IP Zone Blocked", 67: "External D/c", 68: "IP Address Error", 69: "Battle Protocol Error",
	70: "Firewall Triggered", 71: "Teleport Exploit", 72: "Item.dat File Error", 73: "Instance map purged",
	74: "Interserver D/c", 75: "Max Logins Reached", 76: "Event Violation", 77: "Login Error 10 times",
	78: "IP Blocked", 79: "Routing Error, Wait", 80: "Acct Locked", 81: "Log in again later",
	// Big5 "請使用自定義帳號登入" ("log in with a custom account").
	82: "\xbd\xd0\xa8\xcf\xa5\xce\xa6\xdb\xa9\x77\xb8\x71\xb1\x62\xb8\xb9\xb5\x6e\xa4\x4a",
}

const (
	defaultReason = "Connection lost" // 0x2eff88
	defaultCode   = 0xff
)

// namedReasons is the bit set at 0x2f01e4: codes shown with their text.
// Others read "D/c:" and the code.
var namedReasons = [32]byte{0x08, 0x80, 0x0a, 0x80, 0x00, 0x22, 0x68, 0xf4, 0x1f, 0xfa, 0x07,
	31: 0x80}

// DisconnectReason is the text the action-0 handler leaves for the
// disconnect message (DAT_00828128 +0x18). gameState is
// PTR_DAT_004caa18 +4; states 5..16 always show the text.
func DisconnectReason(sub byte, gameState int) []byte {
	text, ok := disconnectReasons[sub]
	code := int(sub)
	if !ok {
		text, code = defaultReason, defaultCode
	}
	if uint(gameState-5) < 12 || namedReasons[code/8]&(1<<(code%8)) != 0 {
		return []byte(text + ":" + strconv.Itoa(code))
	}
	return []byte("D/c:" + strconv.Itoa(code))
}
