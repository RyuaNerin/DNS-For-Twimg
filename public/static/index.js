"use strict";
const chartColor = "#dc3545"; // .text-secondary
const chartColorDNS = "#007bff"; // .text-primary
const chartColorCache = "#17a2b8"; // .text-primary
const fontFamilly = `"Apple SD 산돌고딕 Neo","Apple SD Gothic Neo","나눔 고딕","맑은 고딕","Malgun Gothic","돋움",dotum,sans-serif`;
function bytesToIECSpeed(bytes) {
    if (typeof bytes !== "number")
        return "0 B";
    if (bytes == 0)
        return "0 B";
    var sizes = ["B", "KiB", "MiB", "GiB", "TiB"];
    var i = 0;
    while (bytes > 1000 && i++ < 5) {
        bytes /= 1024;
    }
    return Math.floor(bytes * 100) / 100 + " " + sizes[i] + "/s";
}
function addToRow(parent, elem) {
    let row = document.createElement("div");
    row.className = "row justify-content-center";
    let col = document.createElement("div");
    col.className = "col col-md-8";
    col.append(elem);
    row.append(col);
    parent.append(row);
}
function drawTable(data) {
    let serverDiv = $("#serverDiv");
    let fontSize = parseFloat(window
        .getComputedStyle(document.body, null)
        .getPropertyValue("font-size"));
    let chartDataBest = [];
    let chartDataDefault = [];
    let getData = (host) => {
        if (!(host in data.detail)) {
            chartDataBest.push(0);
            chartDataDefault.push(0);
            return;
        }
        const detail = data.detail[host];
        chartDataBest.push(detail.best.speed || 0);
        chartDataDefault.push(detail.default.speed || 0);
    };
    getData("pbs.twimg.com");
    getData("video.twimg.com");
    let chart = document.createElement("canvas");
    addToRow(serverDiv, chart);
    new Chart(chart, {
        type: "horizontalBar",
        data: {
            datasets: [
                {
                    label: "적용 후",
                    backgroundColor: [chartColorDNS, chartColorDNS],
                    data: chartDataBest,
                },
                {
                    label: "적용 전",
                    backgroundColor: [chartColor, chartColor],
                    data: chartDataDefault,
                },
            ],
            labels: ["이미지", "비디오"],
        },
        options: {
            responsive: true,
            aspectRatio: 0.8,
            //aspectRatio: 0.7, // 전체 표시할 때
            maintainAspectRatio: false,
            layout: {
                padding: {
                    left: 10,
                    right: 10,
                },
            },
            animation: {
                duration: 0,
            },
            hover: {
                animationDuration: 0,
            },
            responsiveAnimationDuration: 0,
            legend: {
                display: true,
                position: "top",
                labels: {
                    fontSize: fontSize,
                    fontFamily: fontFamilly,
                },
            },
            title: {
                fontSize: fontSize * 1.5,
                fontFamily: fontFamilly,
                display: true,
                text: data.updated_at + " 기준",
            },
            tooltips: {
                mode: "nearest",
                xPadding: 10,
                yPadding: 10,
                bodyFontFamily: fontFamilly,
                bodyFontSize: fontSize,
                displayColors: false,
                callbacks: {
                    title: () => "",
                    label: (tooltipItem) => bytesToIECSpeed(tooltipItem.xLabel),
                },
            },
            scales: {
                yAxes: [
                    {
                        ticks: {
                            fontSize: fontSize,
                            fontFamily: fontFamilly,
                        },
                        gridLines: {
                            display: false,
                        },
                    },
                ],
                xAxes: [
                    {
                        ticks: {
                            min: 0,
                            stepSize: 256 * 1024,
                            maxTicksLimit: 5,
                            fontSize: fontSize,
                            fontFamily: fontFamilly,
                            callback: (value) => bytesToIECSpeed(value),
                        },
                    },
                ],
            },
        },
    });
}
$(document).ready(() => {
    $.ajax({
        method: "GET",
        url: "/json.2",
        async: true,
        complete: (r) => {
            if (r.status == 200) {
                const d = JSON.parse(r.responseText);
                if (d)
                    drawTable(d);
            }
        },
    });
    $.ajax({
        method: "GET",
        url: "//test.twimg.ryuar.in",
        timeout: 3000,
        async: true,
        complete: (r) => {
            if (r.status == 200) {
                switch (r.responseText) {
                    case "WITH TWIMG-DNS":
                        $("#with-option").removeClass("invisible");
                        break;
                }
            }
        },
    });
});
