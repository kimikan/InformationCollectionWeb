package utils

import (
	"io/ioutil"
)

var config string

func GetConfigUrl() string {
	if len(config) > 0 {
		return config
	}

	content, err := ioutil.ReadFile("config.txt")
	if err != nil {
		return ""
	}

	config = string(content)
	return config
}
