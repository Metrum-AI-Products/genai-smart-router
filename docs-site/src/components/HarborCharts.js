// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

import React from "react";
import {
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  Legend,
  LinearScale,
  Tooltip,
} from "chart.js";
import { Bar } from "react-chartjs-2";
import styles from "./HarborCharts.module.css";

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip, Legend);

const labels = ["Codex CLI", "Claude Code CLI"];
const reward = [1, 0];
const elapsed = [68, 348];
const totalTokens = [82812, 577592];
const outputTokens = [2598, 22530];

const gridColor = "rgba(255,255,255,0.11)";
const textColor = "#d7d9e0";

function baseOptions(yTitle) {
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
        beginAtZero: true,
      },
    },
  };
}

export default function HarborCharts() {
  return (
    <div className={styles.grid}>
      <section className={styles.panel}>
        <div className={styles.heading}>
          <p>Verifier Reward</p>
          <h3>Outcome by agent</h3>
        </div>
        <div className={styles.chart}>
          <Bar
            options={baseOptions("reward")}
            data={{
              labels,
              datasets: [
                { label: "Reward", data: reward, backgroundColor: "#fe005f" },
              ],
            }}
          />
        </div>
      </section>

      <section className={styles.panel}>
        <div className={styles.heading}>
          <p>Elapsed Time</p>
          <h3>Runtime by agent</h3>
        </div>
        <div className={styles.chart}>
          <Bar
            options={baseOptions("seconds")}
            data={{
              labels,
              datasets: [
                { label: "Elapsed seconds", data: elapsed, backgroundColor: "#465cda" },
              ],
            }}
          />
        </div>
      </section>

      <section className={styles.panelWide}>
        <div className={styles.heading}>
          <p>Token Demand</p>
          <h3>Harbor-reported tokens by agent</h3>
        </div>
        <p className={styles.note}>
          Token totals are from the Harbor job summaries for the current production run. Cache-read tokens are included in total token demand because they affect context pressure and agent loop behavior.
        </p>
        <div className={styles.chart}>
          <Bar
            options={baseOptions("tokens")}
            data={{
              labels,
              datasets: [
                { label: "Total tokens", data: totalTokens, backgroundColor: "#cc28af" },
                { label: "Output tokens", data: outputTokens, backgroundColor: "#ff3132" },
              ],
            }}
          />
        </div>
      </section>
    </div>
  );
}
