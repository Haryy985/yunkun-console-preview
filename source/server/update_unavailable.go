package server

import "net/http"

func updateUnavailable(w http.ResponseWriter, _ *http.Request) {
	writeErr(w, http.StatusServiceUnavailable, "云鲲发布源尚未配置，应用内更新已停用")
}
