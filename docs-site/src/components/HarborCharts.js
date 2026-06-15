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
import styles from "./HarborCharts.module.css";

ChartJS.register(CategoryScale, LinearScale, BarElement, LineElement, PointElement, Tooltip, Legend);

const labels = ["default", "fast", "small", "medium", "high", "big-coder"];
const codexElapsed = [139, 358, 156, 116, 119, 91];
const claudeElapsed = [405, 128, 96, 131, 267, 171];
const codexTotalTokens = [221843, 191344, 265025, 94250, 66505, 52695];
const claudeTotalTokens = [177405, 133276, 152385, 136147, 108237, 119425];
const outputThroughput = [68.86, 68.61, 69.36];
const costLabels = ["All runs", "Codex", "Claude Code"];
const gptCosts = [11.09509, 6.247065, 4.848025];
const opusCosts = [10.203675, 5.80288, 4.400795];
const metrumCosts = [0.1505886, 0.0894065, 0.0611821];

const gridColor = "rgba(255,255,255,0.11)";
const textColor = "#d7d9e0";

function baseOptions(title, yTitle) {
  return {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        labels: { color: textColor, boxWidth: 14, boxHeight: 14 },
      },
      title: { display: false, text: title },
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

export default function HarborCharts() {
  return (
    <div className={styles.grid}>
      <section className={styles.panel}>
        <div className={styles.heading}>
          <p>Elapsed Time</p>
          <h3>Agent runtime by model group</h3>
        </div>
        <div className={styles.chart}>
          <Bar
            options={baseOptions("Agent runtime by model group", "seconds")}
            data={{
              labels,
              datasets: [
                { label: "Codex", data: codexElapsed, backgroundColor: "#fe005f" },
                { label: "Claude Code", data: claudeElapsed, backgroundColor: "#465cda" },
              ],
            }}
          />
        </div>
      </section>

      <section className={styles.panel}>
        <div className={styles.heading}>
          <p>Token Demand</p>
          <h3>Total Harbor tokens by agent and group</h3>
        </div>
        <div className={styles.chart}>
          <Bar
            options={baseOptions("Total Harbor tokens by agent and group", "tokens")}
            data={{
              labels,
              datasets: [
                { label: "Codex", data: codexTotalTokens, backgroundColor: "#cc28af" },
                { label: "Claude Code", data: claudeTotalTokens, backgroundColor: "#9948cb" },
              ],
            }}
          />
        </div>
      </section>

      <section className={styles.panelWide}>
        <div className={styles.heading}>
          <p>Throughput</p>
          <h3>Average upstream output throughput</h3>
        </div>
        <div className={styles.chartSmall}>
          <Line
            options={baseOptions("Average upstream output throughput", "output tokens/sec")}
            data={{
              labels: ["All requests", "Codex", "Claude Code"],
              datasets: [
                {
                  label: "Output tok/s",
                  data: outputThroughput,
                  borderColor: "#ff3132",
                  backgroundColor: "rgba(255,49,50,0.18)",
                  pointBackgroundColor: "#fe005f",
                  tension: 0.32,
                },
              ],
            }}
          />
        </div>
      </section>

      <section className={styles.panelWide}>
        <div className={styles.heading}>
          <p>Cost Savings Using Actual Agents</p>
          <h3>Same successful coding task, benchmarked by endpoint economics</h3>
        </div>
        <p className={styles.note}>
          Both the OpenAI-priced comparison endpoint and the Metrum-routed endpoint are assumed successful on the same Harbor coding task. This chart applies the recorded benchmark token counts to three pricing assumptions.
        </p>
        <div className={styles.chart}>
          <Bar
            options={baseOptions("Cost comparison", "estimated USD")}
            data={{
              labels: costLabels,
              datasets: [
                { label: "GPT 5.5 assumed", data: gptCosts, backgroundColor: "#ff3132" },
                { label: "Opus 4.8 assumed", data: opusCosts, backgroundColor: "#9948cb" },
                { label: "Metrum routed assumed", data: metrumCosts, backgroundColor: "#465cda" },
              ],
            }}
          />
        </div>
      </section>
    </div>
  );
}
