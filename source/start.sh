#!/bin/sh
# 云鲲守护启动脚本（Linux / macOS 原生部署）
#
# 用法：
#   ./start.sh                       前台运行（Ctrl-C 停止）
#   nohup ./start.sh >yunkun.log 2>&1 &   后台常驻
#   ./start.sh -addr :9000           额外参数原样透传给 yunkun
#
# 启动 yunkun 并转发停止信号；异常退出时退避重试。
#
#   0      用户正常停止        → 退出循环
#   75     保留的重启退出码    → 立刻重跑
#   其他   崩溃                → 退避后重跑（1→2→4…最多 60 秒）
#
# 应用内更新已停用；脚本不下载或换装二进制。
set -u

cd "$(dirname "$0")" || exit 1

BIN=./yunkun
[ -x "$BIN" ] || { echo "[yunkun] 找不到可执行文件 $BIN" >&2; exit 1; }

RESTART_CODE=75
MAX_DELAY=60

child=0
stopping=0

# 转发停止信号给后端进程。
#
# 收到停止请求后让后端完成优雅关闭，避免正在执行的任务被直接中断。
forward() {
	stopping=1
	if [ "$child" -ne 0 ]; then
		kill -TERM "$child" 2>/dev/null || true
	fi
}
trap forward INT TERM

delay=1
while :; do
	"$BIN" "$@" &
	child=$!

	# 信号会打断 wait 并让它返回 >128。此时子进程其实还在做优雅关闭，
	# 必须再 wait 一次才能拿到它真正的退出码。
	wait "$child"
	code=$?
	if [ "$code" -gt 128 ]; then
		wait "$child"
		code=$?
	fi
	child=0

	if [ "$stopping" -eq 1 ]; then
		echo "[yunkun] 已停止"
		exit 0
	fi

	case "$code" in
		0)
			echo "[yunkun] 正常退出"
			exit 0
			;;
		"$RESTART_CODE")
			echo "[yunkun] 请求重启…"
			delay=1
			;;
		*)
			echo "[yunkun] 异常退出 (code=$code)，${delay}s 后重启" >&2
			sleep "$delay"
			delay=$((delay * 2))
			[ "$delay" -gt "$MAX_DELAY" ] && delay=$MAX_DELAY
			;;
	esac
done
