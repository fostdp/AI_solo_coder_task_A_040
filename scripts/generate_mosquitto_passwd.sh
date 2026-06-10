#!/bin/bash

echo "生成Mosquitto密码文件..."
echo "用户名: admin"
echo "密码: admin123"
echo ""

TEMP_PASSWD=$(mktemp)

docker run --rm -v ${TEMP_PASSWD}:/tmp/passwd eclipse-mosquitto:2.0-openssl \
    mosquitto_passwd -c -b /tmp/passwd admin admin123

if [ $? -eq 0 ]; then
    mkdir -p mosquitto/config
    cp ${TEMP_PASSWD} mosquitto/config/passwd
    rm ${TEMP_PASSWD}
    echo ""
    echo "密码文件已生成: mosquitto/config/passwd"
    ls -la mosquitto/config/passwd
else
    echo "生成密码文件失败"
    rm -f ${TEMP_PASSWD}
    exit 1
fi
