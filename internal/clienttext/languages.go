package clienttext

import (
	"encoding/csv"
	"strconv"
	"strings"
	"sync"

	"wonderland-gonline/internal/localization"
)

var chineseText = sync.OnceValue(func() map[string]string {
	rows, err := csv.NewReader(strings.NewReader(chineseCSV)).ReadAll()
	if err != nil {
		panic(err)
	}
	if len(rows) == 0 || strings.Join(rows[0], ",") != "table,id,field,text,source_record_sha256,target_record_sha256" {
		panic("invalid Chinese text header")
	}
	out := map[string]string{}
	for _, row := range rows[1:] {
		id, err := strconv.ParseUint(row[1], 10, 32)
		if err != nil {
			panic(err)
		}
		key := localization.Key(row[0], uint32(id), row[2])
		if _, ok := out[key]; ok {
			panic("duplicate Chinese text key")
		}
		out[key] = row[3]
	}
	return out
})

func NewCatalogs() map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, code := range []string{localization.English, localization.Chinese} {
		out[code] = localization.BaseStrings(code)
	}
	return out
}
func MergeCatalogs(dst, src map[string]map[string]string) {
	for code, values := range src {
		for k, v := range values {
			dst[code][k] = v
		}
	}
}
