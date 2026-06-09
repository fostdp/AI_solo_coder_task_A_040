const ChartModule = (function() {

    function drawConcentrationChart(canvasId, data) {
        const canvas = document.getElementById(canvasId);
        if (!canvas || !data || data.length === 0) return;
        
        const ctx = canvas.getContext('2d');
        const width = canvas.width;
        const height = canvas.height;
        const padding = { top: 30, right: 30, bottom: 40, left: 60 };
        const chartWidth = width - padding.left - padding.right;
        const chartHeight = height - padding.top - padding.bottom;

        ctx.clearRect(0, 0, width, height);

        const concentrations = data.map(d => d.concentration);
        const maxConcentration = Math.max(...concentrations, Config.ALARM_LEVEL1 * 1.2);
        const minConcentration = 0;

        drawGrid(ctx, padding, chartWidth, chartHeight, maxConcentration);
        drawThresholdLines(ctx, padding, chartWidth, chartHeight, maxConcentration);
        drawDataLine(ctx, data, padding, chartWidth, chartHeight, maxConcentration, minConcentration);
        drawAxes(ctx, padding, chartWidth, chartHeight, maxConcentration);
        drawLabels(ctx, padding, chartWidth, chartHeight, data, maxConcentration);
    }

    function drawGrid(ctx, padding, width, height, maxValue) {
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.1)';
        ctx.lineWidth = 1;

        const gridLines = 5;
        for (let i = 0; i <= gridLines; i++) {
            const y = padding.top + (height / gridLines) * i;
            ctx.beginPath();
            ctx.moveTo(padding.left, y);
            ctx.lineTo(padding.left + width, y);
            ctx.stroke();
        }

        const verticalLines = 6;
        for (let i = 0; i <= verticalLines; i++) {
            const x = padding.left + (width / verticalLines) * i;
            ctx.beginPath();
            ctx.moveTo(x, padding.top);
            ctx.lineTo(x, padding.top + height);
            ctx.stroke();
        }
    }

    function drawThresholdLines(ctx, padding, width, height, maxValue) {
        const thresholds = [
            { value: Config.ALARM_LEVEL1, color: 'rgba(255, 255, 0, 0.7)', label: '一级预警' },
            { value: Config.ALARM_LEVEL2, color: 'rgba(255, 165, 0, 0.7)', label: '二级报警' },
            { value: Config.ALARM_LEVEL3, color: 'rgba(255, 0, 0, 0.9)', label: '三级紧急' }
        ];

        thresholds.forEach(threshold => {
            if (threshold.value <= maxValue) {
                const y = padding.top + height - (threshold.value / maxValue) * height;
                
                ctx.setLineDash([5, 5]);
                ctx.strokeStyle = threshold.color;
                ctx.lineWidth = 1.5;
                ctx.beginPath();
                ctx.moveTo(padding.left, y);
                ctx.lineTo(padding.left + width, y);
                ctx.stroke();
                ctx.setLineDash([]);

                ctx.fillStyle = threshold.color;
                ctx.font = '10px Arial';
                ctx.textAlign = 'left';
                ctx.fillText(`${threshold.label} (${threshold.value}%LEL)`, padding.left + 5, y - 3);
            }
        });
    }

    function drawDataLine(ctx, data, padding, width, height, maxValue, minValue) {
        if (data.length < 2) return;

        const gradient = ctx.createLinearGradient(0, padding.top, 0, padding.top + height);
        gradient.addColorStop(0, 'rgba(0, 255, 136, 0.3)');
        gradient.addColorStop(0.5, 'rgba(255, 200, 0, 0.3)');
        gradient.addColorStop(1, 'rgba(255, 0, 0, 0.3)');

        ctx.beginPath();
        ctx.moveTo(padding.left, padding.top + height);

        data.forEach((point, index) => {
            const x = padding.left + (index / (data.length - 1)) * width;
            const normalizedValue = (point.concentration - minValue) / (maxValue - minValue);
            const y = padding.top + height - normalizedValue * height;
            
            if (index === 0) {
                ctx.lineTo(x, y);
            } else {
                ctx.lineTo(x, y);
            }
        });

        ctx.lineTo(padding.left + width, padding.top + height);
        ctx.closePath();
        ctx.fillStyle = gradient;
        ctx.fill();

        ctx.beginPath();
        data.forEach((point, index) => {
            const x = padding.left + (index / (data.length - 1)) * width;
            const normalizedValue = (point.concentration - minValue) / (maxValue - minValue);
            const y = padding.top + height - normalizedValue * height;

            if (index === 0) {
                ctx.moveTo(x, y);
            } else {
                ctx.lineTo(x, y);
            }
        });

        ctx.strokeStyle = '#00ff88';
        ctx.lineWidth = 2;
        ctx.stroke();

        data.forEach((point, index) => {
            const x = padding.left + (index / (data.length - 1)) * width;
            const normalizedValue = (point.concentration - minValue) / (maxValue - minValue);
            const y = padding.top + height - normalizedValue * height;

            let color = '#00ff88';
            if (point.concentration >= Config.ALARM_LEVEL3) {
                color = '#ff0000';
            } else if (point.concentration >= Config.ALARM_LEVEL2) {
                color = '#ffa500';
            } else if (point.concentration >= Config.ALARM_LEVEL1) {
                color = '#ffff00';
            }

            ctx.beginPath();
            ctx.arc(x, y, 3, 0, Math.PI * 2);
            ctx.fillStyle = color;
            ctx.fill();
        });
    }

    function drawAxes(ctx, padding, width, height, maxValue) {
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.5)';
        ctx.lineWidth = 2;

        ctx.beginPath();
        ctx.moveTo(padding.left, padding.top);
        ctx.lineTo(padding.left, padding.top + height);
        ctx.stroke();

        ctx.beginPath();
        ctx.moveTo(padding.left, padding.top + height);
        ctx.lineTo(padding.left + width, padding.top + height);
        ctx.stroke();

        ctx.fillStyle = 'rgba(255, 255, 255, 0.8)';
        ctx.font = '12px Arial';
        ctx.textAlign = 'right';
        ctx.fillText('浓度 (%LEL)', padding.left - 10, padding.top - 10);

        ctx.textAlign = 'center';
        ctx.fillText('时间', padding.left + width / 2, padding.top + height + 30);
    }

    function drawLabels(ctx, padding, width, height, data, maxValue) {
        ctx.fillStyle = 'rgba(255, 255, 255, 0.7)';
        ctx.font = '10px Arial';
        ctx.textAlign = 'right';

        const gridLines = 5;
        for (let i = 0; i <= gridLines; i++) {
            const y = padding.top + (height / gridLines) * i;
            const value = ((gridLines - i) / gridLines) * maxValue;
            ctx.fillText(value.toFixed(1), padding.left - 5, y + 3);
        }

        ctx.textAlign = 'center';
        const timeLabels = 6;
        for (let i = 0; i <= timeLabels; i++) {
            const x = padding.left + (width / timeLabels) * i;
            const dataIndex = Math.floor((i / timeLabels) * (data.length - 1));
            if (data[dataIndex]) {
                const time = new Date(data[dataIndex].timestamp * 1000);
                const timeStr = `${time.getHours().toString().padStart(2, '0')}:${time.getMinutes().toString().padStart(2, '0')}`;
                ctx.fillText(timeStr, x, padding.top + height + 15);
            }
        }
    }

    function drawHealthIndicator(canvasId, healthStatus) {
        const canvas = document.getElementById(canvasId);
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        const width = canvas.width;
        const height = canvas.height;
        const centerX = width / 2;
        const centerY = height / 2;
        const radius = Math.min(width, height) / 2 - 5;

        ctx.clearRect(0, 0, width, height);

        let color = '#00ff88';
        let statusText = '正常';
        let statusDescription = '传感器运行正常';

        if (healthStatus === 'warning') {
            color = '#ffa500';
            statusText = '警告';
            statusDescription = '需要维护';
        } else if (healthStatus === 'error' || healthStatus === 'fault') {
            color = '#ff0000';
            statusText = '故障';
            statusDescription = '传感器故障';
        } else if (healthStatus === 'offline') {
            color = '#666666';
            statusText = '离线';
            statusDescription = '设备离线';
        }

        ctx.beginPath();
        ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
        ctx.fillStyle = `${color}33`;
        ctx.fill();
        ctx.strokeStyle = color;
        ctx.lineWidth = 3;
        ctx.stroke();

        ctx.fillStyle = color;
        ctx.font = 'bold 16px Arial';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(statusText, centerX, centerY - 5);

        ctx.fillStyle = 'rgba(255, 255, 255, 0.7)';
        ctx.font = '10px Arial';
        ctx.fillText(statusDescription, centerX, centerY + 12);
    }

    return {
        drawConcentrationChart: drawConcentrationChart,
        drawHealthIndicator: drawHealthIndicator
    };
})();
