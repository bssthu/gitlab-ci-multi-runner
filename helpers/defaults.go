package helpers

import (
	"strconv"
)

func IsEmpty(data *string) bool {
	return data == nil || *data == ""
}

func StringOrDefault(data *string, def string) string {
	if IsEmpty(data) {
		return def
	} else {
		return *data
	}
}

func NonZeroOrDefault(data *int, def int) int {
	if data == nil || *data <= 0 {
		return def
	} else {
		return *data
	}
}

func BoolOrDefault(data *bool, def bool) bool {
	if data == nil {
		return def
	} else {
		return *data
	}
}

func IntFromStringOrDefault(str string, def int) int {
	if len(str) > 0 {
		i, err := strconv.Atoi(str)
		if err == nil {
			return i
		}
	}
	return def
}
