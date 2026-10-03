package login

import (
	"encoding/binary"
	"math/rand/v2"
	"strconv"

	"wonderland-go/internal/protocol"
)

// ClientVersion is the word the 63/4 send writes after the command
// (0x2d9007).
const ClientVersion = 0x4bb

// itemExport records the original Item.Dat size used for the login token.
const itemExport = "item_data.json"

// Discovery is the send for action 0 (FUN_002c2394 case 0).
func Discovery() []byte { return []byte{protocol.CommandDiscovery} }

// LoginPacket is the 63/4 send (FUN_002c2394, case at 0x2d8e37).
func (f *IDPassword) LoginPacket() []byte {
	return BuildLogin(f.Account.Text, f.Password.Text, f.token())
}

// token is IntToStr of Data\Item.Dat's size, or nothing without the file.
// The size comes exclusively from the item export's source_bytes.
func (f *IDPassword) token() []byte {
	if h, err := readExportHeader(f.Assets.DataPath(itemExport), "items"); err == nil && h.SourceBytes > 0 {
		return []byte(strconv.FormatInt(h.SourceBytes, 10))
	}
	return nil
}

// BuildLogin lays out 63/4: command, version, the trimmed account (at
// most 10 bytes) and password as length-prefixed strings, then the token
// XORed with a random key: its length, the key, and the bytes.
func BuildLogin(account, password, token []byte) []byte {
	acct := trim(account)
	if len(acct) > fieldMaxBytes {
		acct = acct[:fieldMaxBytes]
	}
	key := byte(rand.IntN(0xff))
	enc := make([]byte, len(token))
	for i, c := range token {
		enc[i] = c ^ key
	}
	p := []byte{protocol.CommandLogin, protocol.LoginAuthenticate}
	p = binary.LittleEndian.AppendUint16(p, ClientVersion)
	p = append(p, byte(len(acct)))
	p = append(p, acct...)
	p = append(p, byte(len(password)))
	p = append(p, password...)
	p = append(p, byte(len(enc)), key)
	return append(p, enc...)
}

// ServerDescription is FUN_004a7028, packet 1/9: the server list closes
// and the account form opens. s is the packet after its command byte.
func (f *IDPassword) ServerDescription(s []byte) {
	if len(s) >= 3 {
		f.G.ServerWord = binary.LittleEndian.Uint16(s[1:])
	}
	if f.Servers != nil {
		f.Servers.Hide()
	}
	f.G.ServerFlag = at(s, 4) == 1
	f.G.ServerText = copyStr(s, 5, len(s)-4)
	f.G.LoginStarted = true
	f.Show()
}
