window.onload = function () {
    var horizonalLinePlugin = {
        beforeDraw: function (chartInstance) {
            var yValue;
            var yScale = chartInstance.scales["weight-y-axis"];
            var canvas = chartInstance.chart;
            var ctx = canvas.ctx;
            var index;
            var line;
            var style;

            ctx.save();
            ctx.lineWidth = 1;
            ctx.setLineDash([5, 3]);

            if (chartInstance.options.horizontalLine) {
                for (index = 0; index < chartInstance.options.horizontalLine.length; index++) {
                    line = chartInstance.options.horizontalLine[index];

                    if (!line.style || !line.y)
                        continue;

                    style = line.style;
                    yValue = yScale.getPixelForValue(line.y);


                    if (yValue) {
                        ctx.beginPath();
                        ctx.moveTo(chartInstance.chartArea.left, yValue);
                        ctx.lineTo(chartInstance.chartArea.right, yValue);
                        ctx.strokeStyle = style;
                        ctx.stroke();
                    }

                    if (line.text) {
                        ctx.fillStyle = style;
                        ctx.fillText(line.text, 0, yValue + ctx.lineWidth);
                    }
                }
            }
            ctx.restore();
        }
    };
    Chart.pluginService.register(horizonalLinePlugin);

    var chart = new Chart(document.getElementById("wchart"), {
        type: 'bar',
        data: {
            labels: [],
            datasets: [
                {
                    type: 'line',
                    data: [],
                    label: "Plant Weight",
                    yAxisID: 'weight-y-axis',
                    borderColor: "#205020",
                    backgroundColor: "#408040",
                    fill: false
                },
                {
                    type: 'line',
                    data: [],
                    label: "Average Weight",
                    yAxisID: 'weight-y-axis',
                    borderColor: "#ffa000",
                    backgroundColor: "#ffc040",
                    borderWidth: 1,
                    pointRadius: 0,
                    fill: false
                },
                {
                    type: 'bar',
                    data: [],
                    yAxisID: 'water-y-axis',
                    label: "Watering",
                    borderColor: "#0030a0",
                    backgroundColor: "#1060c0",
                    fill: false
                }
            ]
        },
        options: {
            horizontalLine: [],
            elements: {
                line: {
                    cubicInterpolationMode: 'monotone'
                }
            },
            scales: {
                xAxes: [{
                    id: 'hour-x-axis',
                    offset: false,
                    ticks: {
                        maxTicksLimit: 48,
                        maxRotation: 0
                    },
                    gridLines: {
                        offsetGridLines: false
                    }
                }],
                yAxes: [{
                    id: 'water-y-axis',
                    type: 'linear',
                    position: 'left'
                },{
                    id: 'weight-y-axis',
                    type: 'linear',
                    position: 'right'
                }]
            }
        }
    });

    var minchart = new Chart(document.getElementById("minchart"), {
        type: 'bar',
        data: {
            labels: [],
            datasets: [
                {
                    type: 'line',
                    data: [],
                    label: "Plant Weight",
                    yAxisID: 'weight-y-axis',
                    borderColor: "#205020",
                    backgroundColor: "#408040",
                    fill: false
                }
            ]
        },
        options: {
            elements: {
                line: {
                    cubicInterpolationMode: 'monotone'
                }
            },
            scales: {
                xAxes: [{
                    id: 'min-x-axis',
                    offset: false,
                    ticks: {
                        maxTicksLimit: 48,
                        maxRotation: 0
                    },
                    gridLines: {
                        offsetGridLines: false
                    }
                }],
                yAxes: [{
                    id: 'weight-y-axis',
                    type: 'linear',
                    position: 'right'
                }]
            }
        }
    });

    function getData() {
        var xhttp = new XMLHttpRequest();
        xhttp.onreadystatechange = function () {
            if (this.readyState == 4 && this.status == 200) {
                var resp = JSON.parse(xhttp.responseText);
                var data = resp.data;
                var len = data.weight.length;
                var start = (data.time + 1 - (len % 24) + 24) % 24;
                var iw = 0;
                var h;
                var avg = 0;
                var count = 0;
                var i, j, w;
                for (i = 0; i < len; ++i) {
                    w = data.water[i];
                    h = (start + i) % 24;
                    chart.data.labels.push(h);
                    chart.data.datasets[0].data.push(data.weight[i]);
                    chart.data.datasets[2].data.push(w / 1000);
                    avg += data.weight[i];
                    ++count;
                    if (w > 0) {
                        // fill average data
                        avg /= count;
                        for (j = 0; j < count; ++j)
                            chart.data.datasets[1].data.push(avg);
                        avg = 0;
                        count = 0;
                    }
                }

                if (count > 0) {
                    avg /= count;
                    for (j = 0; j < count; ++j)
                        chart.data.datasets[1].data.push(avg);
                }

                var col = { range: '#c0c0c0', low: '#ff0000', dst: '#40b000' };


                var config = resp.config;
                chart.options.scales.yAxes[0].ticks.min = 0;
                chart.options.scales.yAxes[0].ticks.max = Math.ceil(config.max / 1000);
                chart.options.scales.yAxes[1].ticks.suggestedMin = Math.floor((config.low - config.range * 2) / 10) * 10;
                chart.options.scales.yAxes[1].ticks.suggestedMax = Math.ceil((config.dst + config.range * 2) / 10) * 10;

                chart.options.horizontalLine.push({y: config.dst-config.range, style: col.range});
                chart.options.horizontalLine.push({y: config.dst+config.range, style: col.range});
                chart.options.horizontalLine.push({y: config.low, style: col.low});
                chart.options.horizontalLine.push({y: config.dst, style: col.dst});

                chart.update();

                var mindata = resp.mindata;
                var mlen = mindata.weight ? mindata.weight.length : 0;
                var minstart = (mindata.time + 1 - (mlen % 60) + 60) % 60;
                var min;
                for (i = 0; i < mlen; ++i) {
                    min = (minstart + i) % 60;
                    minchart.data.labels.push(min);
                    // minchart.data.datasets[0].data.push(mindata.moisture[i]);
                    // 4052 is weight value with no load
                    minchart.data.datasets[0].data.push(mindata.weight[i]);
                }

                minchart.options.scales.yAxes[0].ticks.suggestedMin = Math.floor((config.low - config.range * 2) / 10) * 10;
                minchart.options.scales.yAxes[0].ticks.suggestedMax = Math.ceil((config.dst + config.range * 2) / 10) * 10;

                minchart.update();
            }
        };
        xhttp.open("GET", "/data", true);
        xhttp.send();
    }
    getData();
};
