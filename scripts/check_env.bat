@echo off
echo ==========================================
echo 智慧城市燃气监测系统 - 启动验证脚本
echo ==========================================
echo.

echo [1/6] 检查Docker环境...
docker --version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker未安装或未启动
    pause
    exit /b 1
)
echo [OK] Docker已就绪
echo.

echo [2/6] 检查Docker Compose...
docker compose version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker Compose未安装
    pause
    exit /b 1
)
echo [OK] Docker Compose已就绪
echo.

echo [3/6] 检查端口占用...
netstat -ano | findstr ":8080" >nul 2>&1
if not errorlevel 1 echo [WARNING] 端口8080可能被占用
netstat -ano | findstr ":1883" >nul 2>&1
if not errorlevel 1 echo [WARNING] 端口1883可能被占用
netstat -ano | findstr ":5432" >nul 2>&1
if not errorlevel 1 echo [WARNING] 端口5432可能被占用
netstat -ano | findstr ":8086" >nul 2>&1
if not errorlevel 1 echo [WARNING] 端口8086可能被占用
netstat -ano | findstr ":9090" >nul 2>&1
if not errorlevel 1 echo [WARNING] 端口9090可能被占用
echo [OK] 端口检查完成
echo.

echo [4/6] 检查必需文件...
if not exist "docker-compose.yml" (
    echo [ERROR] docker-compose.yml不存在
    pause
    exit /b 1
)
if not exist ".env" (
    echo [WARNING] .env文件不存在，将使用默认值
)
if not exist "backend\Dockerfile" (
    echo [ERROR] backend\Dockerfile不存在
    pause
    exit /b 1
)
if not exist "simulator\Dockerfile" (
    echo [ERROR] simulator\Dockerfile不存在
    pause
    exit /b 1
)
echo [OK] 必需文件检查完成
echo.

echo [5/6] 创建数据目录...
if not exist "mosquitto\data" mkdir mosquitto\data
if not exist "mosquitto\log" mkdir mosquitto\log
if not exist "influxdb\config" mkdir influxdb\config
if not exist "influxdb\scripts" mkdir influxdb\scripts
if not exist "prometheus\rules" mkdir prometheus\rules
echo [OK] 数据目录创建完成
echo.

echo [6/6] 检查Mosquitto密码文件...
if not exist "mosquitto\config\passwd" (
    echo [ERROR] mosquitto\config\passwd不存在
    echo 请手动创建密码文件:
    echo   docker run --rm -it eclipse-mosquitto:2.0-openssl mosquitto_passwd -c /tmp/passwd admin
    echo   docker cp ^<container_id^>:/tmp/passwd ./mosquitto/config/passwd
    pause
    exit /b 1
)
echo [OK] Mosquitto密码文件检查完成
echo.

echo ==========================================
echo 环境检查完成！
echo ==========================================
echo.
echo 下一步操作:
echo   1. 构建镜像: make build
echo   2. 启动服务: make up 或 make up-all
echo   3. 查看状态: make ps
echo   4. 访问前端: http://localhost:8080
echo.
echo 快速启动命令:
echo   docker compose up -d
echo.
pause
