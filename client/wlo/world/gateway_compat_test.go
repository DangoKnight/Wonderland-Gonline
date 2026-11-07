package world

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"
)

// Captured from the pre-optimization RGB565 renderer at 32 phases over its
// complete loop. These independent reference pixels preserve the visual effect.
func TestGatewayLightReferencePixels(t *testing.T) {
	expected := []string{
		"a339fd3283da2c5e505fce0e84d6cf7534b910057cb5fa65fc54bcd084c6e316",
		"a82636caaa0bc138761a823eab19be863cc114c7c761e89b18b726ab20dc0bac",
		"c0dd40f18a8af7f0f52dc4dd30ced30a9240d7b868a74074a7ada0cf2d3e3c21",
		"07a4435721a5578bd96b6f47ca6f0d9422303dbf803993f7f1421d7852469558",
		"aa4dccd7662fcaf482bbdf17185f7bdccc9e44320d248c8831208ccab21e287f",
		"11d153973c2e1000c8ad9616de19648b68a0ef7c2f48d6e2290a0cbaac62b9f6",
		"f5ebe4d12b9e04a62fa114550c40074ac7c119541a5bb6cacf2e81dde49eb35b",
		"6728c31a6f9a9e6a89f5ab2271d3f4ee4cafe2a64121254672b5827b196fee29",
		"a7cd77746f25ec6acc0da3467647ca49e4f38d9b5a35b974fd7aa493c93c732a",
		"4f2b9e90d6d60b63f2102a01718bd5ea25b74970f7db65dba520ef7e6d1a8e1e",
		"bd54ee0259e2389c4c20b75982619a75596722402973121f1ed8861dc43f9724",
		"e5ca2c0a2c363da1f794e6dc3c749ee6083e2808d453b92358ce8b5de31a7281",
		"ba10161e3030f4dfa2c66b177827aad25ced8945aa781e70fec81e4c3b8c8418",
		"d23f2e6c8380a486bf004333548076f3c9c347ae6eba13a8e325bb45118f1a11",
		"a36d4292f4c93e5fed2632ed1dad13301ab68e29d1db05bbe53e2eaf70ef8ddd",
		"a5ee6f607af83f15fda4420dc20a65d4dd8dd2a4f0ff380706f07e6133205046",
		"03730a8d3469650b56aef0d89aa9e4288d62f7e46136a1a01c58ffa1ef8219ff",
		"5503468221e1024319183939fdcf8b6cbfee019ddc0ec8364d7b19c1aafe06e6",
		"aa0ff1500f057c94cc900b61a8d566da6b11e6ff6ede779db6b66ab72001f7d2",
		"1adb40361e35b9bf2499143805e1c5ac8d5555148c25811ac8e8da6f92476fca",
		"022969a68843e12a4d1e16b9835acabd5420102661384bed1ef9b1de48818402",
		"0eca46ff6ecb7137a001b5a2bae78443740352cd7d3bf09ed2dd5cfa189bca9c",
		"8f8a781420afe986e7619ba726ccd1520841f7bfabcaaf8f7071b18dfa02ede6",
		"2e6992313e85a2a0f4036f9a6885b60c65fda5435a46fec74b3c46a8d6d0b682",
		"e1e66192b37c97c8e1e7cc0e153303364e2062c53de7faf1dd48e9755401e5ec",
		"de4715fa5f012bbc20fe0138b5dc7177894d9038368a195cfb09ad252ea371fa",
		"381ca7c3d465b1170320c5b201ce05256030be570b240019f3bde2420a027183",
		"0ff5861a4649142a04e223d42b2ccbba0fc8dda48575c6befe229d112513e149",
		"766b2c8a68c326fe35fb20fd3b9f3cf7526285ee1c349014353e93461ce21899",
		"d02bc0aae8ec52bb5ce804de43e289a75cf96487fb4f974801c759ddb9092d01",
		"8b9ea8c8335c0c8c9846b0f936baa93247e4448b250fd11f5b2a2d80ef94a9de",
		"e18bd23cafabda0bac3783dc3130254f7a8bef733b4b0393a9c084d04d08e241",
	}
	p := newGatewayLight()
	raw := make([]byte, 2*gatewayWidth*gatewayHeight)
	for frame, want := range expected {
		p.render(time.Duration(frame) * gatewayPeriod / time.Duration(len(expected)))
		for i, pixel := range p.pixels.Pix {
			binary.LittleEndian.PutUint16(raw[i*2:], pixel)
		}
		sum := sha256.Sum256(raw)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Fatalf("gateway phase %d changed reference pixels: %s", frame, got)
		}
	}
}
