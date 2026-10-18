package server

import "net/http"

func updateUnavailable(w http.ResponseWriter, _ *http.Request) {
	writeErr(w, http.StatusServiceUnavailable, "应用内自动替换未启用，请从云鲲 GitHub 发布页下载对应版本并手动更新")
}
