package beater

import "github.com/elastic/elastic-agent-libs/mapstr"

// reusable function to update a map with the key and value if value is not empty ("")
func updateMapString(key string, value string, m mapstr.M) {
	if value != "" {
		m.Put(EventFieldMap[key], value)
	}
}

// reusable function to update a map with the key and value if value is not 0
func updateMapNumber(key string, value uint32, m mapstr.M) {
	if value != 0 {
		m.Put(EventFieldMap[key], value)
	}
}
