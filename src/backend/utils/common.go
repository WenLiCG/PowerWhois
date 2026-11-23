package utils

import (
	"github.com/go-resty/resty/v2"
)

func HttpRetryChecker(resp *resty.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp.StatusCode() != 200 {
		return true
	}
	return false
}
