package assets

import "testing"

func TestPetVoucherTable(t *testing.T) {
	for _, data := range []string{`null`, `[]`, `[{"item_id":0,"pet_id":1}]`, `[{"item_id":1,"pet_id":0}]`, `[{"item_id":1,"pet_id":2},{"item_id":1,"pet_id":3}]`, `[{"item_id":65536,"pet_id":1}]`, `[`} {
		if v, err := ParsePetVouchers([]byte(data)); err == nil || v != nil {
			t.Fatal("accepted invalid table", data)
		}
	}
	v, err := ParsePetVouchers([]byte(`[{"item_id":30068,"pet_id":14719}]`))
	if err != nil || v[30068] != 14719 {
		t.Fatal(v, err)
	}
}
