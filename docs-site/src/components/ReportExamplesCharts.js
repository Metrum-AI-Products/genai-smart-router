// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

import React from "react";
import {
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  Tooltip,
} from "chart.js";
import { Bar, Line } from "react-chartjs-2";
import styles from "./ReportExamplesCharts.module.css";

ChartJS.register(CategoryScale, LinearScale, BarElement, LineElement, PointElement, Tooltip, Legend);

const days = ["Jun 17", "Jun 18", "Jun 19", "Jun 20", "Jun 21", "Jun 22"];
const calls = [2277, 2773, 2782, 1725, 1333, 3142];
const routerCost = [14.62, 45.2, 56.54, 31.75, 70.47, 135.54];
const referenceCost = [425.59, 355.1, 747.69, 454.06, 1013.62, 613.5];
const savings = [410.97, 309.9, 691.15, 422.31, 943.15, 477.97];
const latency = [13390, 11782, 11966, 10665, 7765, 8224];
const callerLabels = ["Team A", "Team B", "Team C", "Team D", "Team E", "Team F"];
const callerSavings = [897.19, 820.44, 237.09, 206.47, 101.21, 164.93];
const callerLatency = [13442, 7849, 16988, 10643, 5363, 14971];
const endpointLabels = ["Endpoint A", "Endpoint B", "Endpoint C", "Endpoint D", "Endpoint E", "Endpoint F", "Endpoint G", "Endpoint H"];
const endpointDuration = [42969, 36077, 25410, 22855, 21015, 20581, 20572, 19272];
const endpointFallbacks = [1, 47, 3, 0, 30, 111, 0, 0];

const gridColor = "rgba(255,255,255,0.11)";
const textColor = "#d7d9e0";

function options(yTitle) {
  return {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        labels: { color: textColor, boxWidth: 14, boxHeight: 14 },
      },
      tooltip: {
        backgroundColor: "#08080a",
        borderColor: "#cc28af",
        borderWidth: 1,
        titleColor: "#ffffff",
        bodyColor: "#d7d9e0",
      },
    },
    scales: {
      x: {
        ticks: { color: textColor },
        grid: { color: "transparent" },
      },
      y: {
        title: { display: true, text: yTitle, color: textColor },
        ticks: { color: textColor },
        grid: { color: gridColor },
      },
    },
  };
}

function heading(kicker, title) {
  return (
    <div className={styles.heading}>
      <p>{kicker}</p>
      <h3>{title}</h3>
    </div>
  );
}

export default function ReportExamplesCharts() {
  return (
    <div className={styles.grid}>
      <section className={styles.panel}>
        {heading("Traffic", "Daily request volume")}
        <div className={styles.chart}>
          <Bar
            options={options("requests")}
            data={{
              labels: days,
              datasets: [{ label: "Calls", data: calls, backgroundColor: "#fe005f" }],
            }}
          />
        </div>
      </section>

      <section className={styles.panel}>
        {heading("Savings", "Router cost vs reference baseline")}
        <div className={styles.chart}>
          <Bar
            options={options("USD")}
            data={{
              labels: days,
              datasets: [
                { label: "Reference cost", data: referenceCost, backgroundColor: "#9948cb" },
                { label: "Router cost", data: routerCost, backgroundColor: "#465cda" },
                { label: "Savings", data: savings, backgroundColor: "#ff3132" },
              ],
            }}
          />
        </div>
      </section>

      <section className={styles.panelWide}>
        {heading("User Experience", "Average latency by day")}
        <div className={styles.chartSmall}>
          <Line
            options={options("milliseconds")}
            data={{
              labels: days,
              datasets: [
                {
                  label: "Avg latency",
                  data: latency,
                  borderColor: "#ff3132",
                  backgroundColor: "rgba(255,49,50,0.18)",
                  pointBackgroundColor: "#fe005f",
                  tension: 0.28,
                },
              ],
            }}
          />
        </div>
      </section>

      <section className={styles.panel}>
        {heading("Caller Cohorts", "Savings by anonymized team")}
        <div className={styles.chart}>
          <Bar
            options={options("USD")}
            data={{
              labels: callerLabels,
              datasets: [{ label: "Savings", data: callerSavings, backgroundColor: "#cc28af" }],
            }}
          />
        </div>
      </section>

      <section className={styles.panel}>
        {heading("Caller Cohorts", "Average latency by anonymized team")}
        <div className={styles.chart}>
          <Bar
            options={options("milliseconds")}
            data={{
              labels: callerLabels,
              datasets: [{ label: "Avg latency", data: callerLatency, backgroundColor: "#465cda" }],
            }}
          />
        </div>
      </section>

      <section className={styles.panelWide}>
        {heading("Upstream Performance", "Slowest anonymized upstream endpoints")}
        <div className={styles.chart}>
          <Bar
            options={options("milliseconds / fallback count")}
            data={{
              labels: endpointLabels,
              datasets: [
                { label: "Avg upstream duration ms", data: endpointDuration, backgroundColor: "#fe005f" },
                { label: "Fallbacks", data: endpointFallbacks, backgroundColor: "#9948cb" },
              ],
            }}
          />
        </div>
      </section>
    </div>
  );
}
