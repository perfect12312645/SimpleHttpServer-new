#!/bin/bash
set -e  # 遇到错误立即退出

# 获取脚本自身的绝对路径（解决符号链接、相对路径执行问题）
SCRIPT_PATH=$(readlink -f "${BASH_SOURCE[0]}")
# 获取脚本所在目录
SCRIPT_DIR=$(dirname "${SCRIPT_PATH}")
# 可执行文件路径 = 脚本目录/SimpleHttpServer
BIN_PATH="${SCRIPT_DIR}/SimpleHttpServer"
BIN_DIR=$(dirname "${BIN_PATH}")

# ===================== 配置变量（可修改以下默认值）=====================
# 服务名（固定为SimpleHttpServer）
SERVICE_NAME="SimpleHttpServer"

# 核心配置变量（默认值与服务一致，可按需修改）
PORT="18181"                # 端口（-P/--port）
USERNAME="admin"            # 用户名（-u/--username）
PASSWORD="admin@123"        # 密码（-p/--password）
MAX_SIZE="20"               # 最大上传文件大小(GB)（-M/--max-size）
UPLOAD_DIR="uploads"        # 上传目录名（-d/--dir）可以为绝对路径，也可相对路径
CHUNK_SIZE="5"              # 分块大小(MB)（-c/--chunk）

# ===================== 端口检测函数（优化版：区分自身/其他进程占用）=====================
# 检测端口是否为合法数字（1-65535）
check_port_valid() {
    local port=$1
    if ! [[ "$port" =~ ^[0-9]+$ ]]; then
        echo -e "\033[31m错误：端口 '$port' 不是有效数字！\033[0m"
        echo "请修改脚本中 PORT 变量为 1-65535 之间的数字后重新执行。"
        return 1
    fi
    if [ "$port" -lt 1 ] || [ "$port" -gt 65535 ]; then
        echo -e "\033[31m错误：端口 '$port' 超出范围（1-65535）！\033[0m"
        echo "请修改脚本中 PORT 变量为 1-65535 之间的数字后重新执行。"
        return 1
    fi
    return 0
}

# 检测占用端口的PID，并判断是否为当前SimpleHttpServer服务
get_port_owner_pid() {
    local port=$1
    local pid=""
    # 优先用ss命令（Linux主流）
    if command -v ss >/dev/null 2>&1; then
        pid=$(ss -tulnp | grep -E ":$port " | awk '{print $7}' | cut -d',' -f1 | sed 's/pid=//g' | grep -E '^[0-9]+$')
    elif command -v netstat >/dev/null 2>&1; then
        pid=$(netstat -tulnp | grep -E ":$port " | awk '{print $7}' | cut -d'/' -f1 | grep -E '^[0-9]+$')
    fi
    # 判断PID是否属于SimpleHttpServer
    if [ -n "$pid" ]; then
        local proc_name=$(ps -p "$pid" -o comm= 2>/dev/null)
        if [ "$proc_name" = "SimpleHttpServer" ]; then
            echo "self"  # 端口被自身服务占用
            return
        fi
    fi
    echo "$pid"  # 返回其他进程PID（空则未占用）
}

# 检测端口是否被占用（排除自身服务）
check_port_used() {
    local port=$1
    # 优先使用ss命令（Linux主流），兼容netstat
    if command -v ss >/dev/null 2>&1 || command -v netstat >/dev/null 2>&1; then
        local owner=$(get_port_owner_pid "$port")
        if [ "$owner" = "self" ]; then
            echo -e "\033[33m提示：端口 $port 被当前SimpleHttpServer服务占用（允许重复执行）\033[0m"
            return 1  # 视为未占用（允许继续）
        elif [ -n "$owner" ]; then
            echo -e "\033[31m错误：端口 $port 被进程PID=$owner 占用（非当前服务）！\033[0m"
            echo "请修改脚本中 PORT 变量为未被占用的端口后重新执行。"
            return 0  # 端口被其他进程占用
        fi
    else
        echo -e "\033[33m警告：未找到ss/netstat命令，跳过端口占用检测！\033[0m"
        return 1  # 跳过检测
    fi
    return 1  # 端口未被占用
}

# 执行端口检测（失败则退出）
run_port_check() {
    echo "===== 端口检测 ====="
    # 第一步：校验端口合法性
    if ! check_port_valid "$PORT"; then
        exit 1
    fi
    # 第二步：检测端口是否被占用（排除自身）
    if check_port_used "$PORT"; then
        exit 1
    fi
    echo "端口检测通过：$PORT（合法且未被其他进程占用）"
}

# ===================== 主逻辑 =====================
# 检查是否为root用户（操作systemd需要root权限）
if [ "$(id -u)" -ne 0 ]; then
    echo -e "\033[31m错误：请使用root用户执行此脚本（sudo ./setup_SimpleHttpServer.sh）\033[0m"
    exit 1
fi

# 执行端口检测
run_port_check

# 检查可执行文件是否存在
if [ ! -f "${BIN_PATH}" ]; then
    echo -e "\033[31m错误：未在脚本目录找到可执行文件！\033[0m"
    echo "脚本目录：${SCRIPT_DIR}"
    echo "期望的可执行文件路径：${BIN_PATH}"
    exit 1
fi

# 检查可执行文件是否有执行权限
if [ ! -x "${BIN_PATH}" ]; then
    echo -e "\033[33m警告：可执行文件缺少执行权限，自动添加...\033[0m"
    chmod +x "${BIN_PATH}"
fi

# 定义上传目录绝对路径（移到主逻辑内，避免作用域问题）
if [[ "${UPLOAD_DIR}" =~ ^/ ]]; then
    # 如果UPLOAD_DIR是绝对路径，直接使用
    UPLOAD_FULL_PATH="${UPLOAD_DIR}"
else
    # 相对路径则拼接可执行文件目录
    UPLOAD_FULL_PATH="${BIN_DIR}/${UPLOAD_DIR}"
fi


# 生成/覆盖systemd服务配置文件
echo -e "\n===== 生成/更新systemd服务文件 ====="
cat > /etc/systemd/system/${SERVICE_NAME}.service << EOF
[Unit]
Description=SimpleHttpServer - 文件上传服务
After=network.target
Documentation=man:SimpleHttpServer(1)

[Service]
Type=simple
WorkingDirectory=${BIN_DIR}
# 启动命令：引用配置变量，拼接所有参数
ExecStart=${BIN_PATH} \
  --port ${PORT} \
  --username ${USERNAME} \
  --password ${PASSWORD} \
  --max-size ${MAX_SIZE} \
  --dir ${UPLOAD_DIR} \
  --chunk ${CHUNK_SIZE}
Restart=on-failure  # 服务崩溃时自动重启
RestartSec=5s       # 重启间隔5秒
StandardOutput=journal+console  # 日志输出到journalctl
StandardError=journal+console

[Install]
WantedBy=multi-user.target
EOF

# 重载systemd配置
echo -e "\n===== 重载systemd配置 ====="
systemctl daemon-reload

# 启动/重启服务（重复执行时自动重启使新配置生效）
echo -e "\n===== 启动/重启服务 ====="
# enable --now：确保开机自启，同时启动服务；restart：如果服务已运行则重启
systemctl enable --now ${SERVICE_NAME}
systemctl restart ${SERVICE_NAME}

# 检查服务状态
echo -e "\n===== 检查服务状态 ====="
sleep 2  # 等待服务启动/重启
if systemctl is-active --quiet ${SERVICE_NAME}; then
    echo -e "\033[32m服务启动成功！\033[0m"
else
    echo -e "\033[31m服务启动失败！\033[0m"
    echo "请执行以下命令查看错误日志：journalctl -u ${SERVICE_NAME} -f"
    exit 1
fi


# 输出成功信息
echo -e "\n===================== 配置完成 ====================="
echo "服务名：${SERVICE_NAME}"
echo -e "访问地址：\033[1m\033[34mhttp://[服务器IP]:${PORT}\033[0m"

echo "用户名：${USERNAME}"
echo "密码：${PASSWORD}"
echo "可执行文件路径：${BIN_PATH}"
echo "上传目录（绝对路径）：${UPLOAD_FULL_PATH}"
echo "日志文件路径：${BIN_DIR}/logs"
echo "启动命令：${BIN_PATH} --port ${PORT} --username ${USERNAME} --password ${PASSWORD} --max-size ${MAX_SIZE} --dir ${UPLOAD_DIR} --chunk ${CHUNK_SIZE}"
echo -e "\n常用命令："
echo "  查看日志：journalctl -u ${SERVICE_NAME} -f"
echo "  停止服务：systemctl stop ${SERVICE_NAME}"
echo "  重启服务：systemctl restart ${SERVICE_NAME}"
echo "  查看状态：systemctl status ${SERVICE_NAME}"
echo "  修改配置后：直接重新执行本脚本即可自动更新！"