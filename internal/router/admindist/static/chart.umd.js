const chartTooltip = (() => {
  let tooltip = null;
  let owner = null;
  let globalListeners = false;

  function hide(requester) {
    if (requester && owner && owner !== requester) return;
    if (tooltip) tooltip.hidden = true;
    if (!requester || owner === requester) owner = null;
  }

  function removeDuplicates() {
    const tooltips = Array.from(document.querySelectorAll(".chart-tooltip"));
    tooltips.forEach((item, index) => {
      if (index > 0) item.remove();
    });
    return tooltips[0] || null;
  }

  function ensure() {
    if (!tooltip || !tooltip.isConnected) {
      tooltip = removeDuplicates();
      if (!tooltip) {
        tooltip = document.createElement("div");
        tooltip.className = "chart-tooltip";
        tooltip.hidden = true;
        document.body.appendChild(tooltip);
      }
    } else {
      tooltip = removeDuplicates() || tooltip;
    }
    attachGlobalListeners();
    return tooltip;
  }

  function attachGlobalListeners() {
    if (globalListeners) return;
    globalListeners = true;
    document.addEventListener("pointerdown", () => hide(), true);
    document.addEventListener("scroll", () => hide(), true);
    window.addEventListener("blur", () => hide());
    window.addEventListener("resize", () => hide());
    document.addEventListener("visibilitychange", () => {
      if (document.hidden) hide();
    });
    document.addEventListener("keydown", event => {
      if (event.key === "Escape") hide();
    });
  }

  function show(chart, html, x, y) {
    const item = ensure();
    owner = chart;
    item.hidden = false;
    item.innerHTML = html;
    item.style.left = `${x + 12}px`;
    item.style.top = `${y + 12}px`;
  }

  window.hideChartTooltip = () => hide();
  return { ensure, hide, show };
})();

window.Chart = class {
  constructor(canvas, config) {
    this.canvas = canvas;
    this.config = config || {};
    this.points = [];
    this.hidden = new Set();
    this.onMove = this.onMove.bind(this);
    this.onLeave = this.onLeave.bind(this);
    this.canvas.addEventListener("mousemove", this.onMove);
    this.canvas.addEventListener("pointerleave", this.onLeave);
    this.canvas.addEventListener("mouseleave", this.onLeave);
    chartTooltip.ensure();
    this.draw();
  }

  destroy() {
    this.canvas.removeEventListener("mousemove", this.onMove);
    this.canvas.removeEventListener("pointerleave", this.onLeave);
    this.canvas.removeEventListener("mouseleave", this.onLeave);
    chartTooltip.hide(this);
    const legend = this.legendElement();
    if (legend) legend.remove();
  }

  draw() {
    chartTooltip.hide(this);
    const ctx = this.canvas.getContext("2d");
    const width = this.canvas.clientWidth || 320;
    const height = this.canvas.clientHeight || 180;
    const ratio = window.devicePixelRatio || 1;
    this.canvas.width = width * ratio;
    this.canvas.height = height * ratio;
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.clearRect(0, 0, width, height);
    this.points = [];

    const options = this.config.options || {};
    const datasets = ((this.config.data && this.config.data.datasets) || []).filter((_, index) => !this.hidden.has(index));
    const labels = (this.config.data && this.config.data.labels) || [];
    this.renderLegend();

    const values = datasets.flatMap((set) => set.data || []).map(Number).filter(Number.isFinite);
    const max = Math.max(1, ...values);
    const plot = { left: 50, top: 18, right: width - 12, bottom: height - 40 };
    const textColor = options.textColor || "#555b67";
    const gridColor = options.gridColor || "#cc28af";
    const yUnit = options.yAxis && options.yAxis.unit;
    const format = options.formatValue || ((value) => String(value));

    ctx.font = "11px MetrumMono, ui-monospace, monospace";
    ctx.fillStyle = textColor;
    ctx.strokeStyle = gridColor;
    ctx.lineWidth = 1;

    for (let i = 0; i <= 3; i++) {
      const y = plot.bottom - ((plot.bottom - plot.top) * i) / 3;
      const value = (max * i) / 3;
      ctx.globalAlpha = i === 0 ? 1 : 0.42;
      ctx.beginPath();
      ctx.moveTo(plot.left, y);
      ctx.lineTo(plot.right, y);
      ctx.stroke();
      ctx.globalAlpha = 1;
      ctx.fillText(format(value, yUnit, false), 4, y + 4);
    }

    if (!values.length) {
      ctx.fillText("No data for selected filters", plot.left, plot.top + 22);
      this.drawAxisLabels(ctx, plot, width, height, textColor);
      return;
    }

    datasets.forEach((set, setIndex) => {
      const data = (set.data || []).map(Number);
      const color = set.borderColor || set.backgroundColor || ["#cc28af", "#ff3132", "#465cda"][setIndex % 3];
      ctx.strokeStyle = color;
      ctx.fillStyle = color;
      ctx.lineWidth = 2;
      ctx.beginPath();
      data.forEach((value, index) => {
        const x = plot.left + (index * Math.max(1, plot.right - plot.left)) / Math.max(1, data.length - 1);
        const y = plot.bottom - (value / max) * Math.max(1, plot.bottom - plot.top);
        if (index === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
        this.points.push({ x, y, value, label: labels[index] || "", series: set.label || "", unit: set.unit || yUnit || "", color });
      });
      ctx.stroke();
      data.forEach((value, index) => {
        const x = plot.left + (index * Math.max(1, plot.right - plot.left)) / Math.max(1, data.length - 1);
        const y = plot.bottom - (value / max) * Math.max(1, plot.bottom - plot.top);
        ctx.beginPath();
        ctx.arc(x, y, 2.8, 0, Math.PI * 2);
        ctx.fill();
      });
    });

    this.drawAxisLabels(ctx, plot, width, height, textColor);
  }

  drawAxisLabels(ctx, plot, width, height, textColor) {
    const options = this.config.options || {};
    const xAxis = options.xAxis || {};
    const yAxis = options.yAxis || {};
    ctx.fillStyle = textColor;
    ctx.font = "11px MetrumMono, ui-monospace, monospace";
    const xLabel = [xAxis.label, xAxis.unit].filter(Boolean).join(" · ");
    const yLabel = [yAxis.label, yAxis.unit].filter(Boolean).join(" · ");
    if (xLabel) ctx.fillText(xLabel, plot.left, height - 12);
    if (yLabel) {
      ctx.save();
      ctx.translate(12, plot.bottom - 4);
      ctx.rotate(-Math.PI / 2);
      ctx.fillText(yLabel, 0, 0);
      ctx.restore();
    }
  }

  renderLegend() {
    const datasets = (this.config.data && this.config.data.datasets) || [];
    let legend = this.legendElement();
    if (!legend) {
      legend = document.createElement("div");
      legend.className = "chart-legend";
      this.canvas.insertAdjacentElement("afterend", legend);
    }
    legend.innerHTML = "";
    datasets.forEach((set, index) => {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "legend-item";
      item.setAttribute("aria-pressed", String(!this.hidden.has(index)));
      item.innerHTML = `<span style="background:${set.borderColor || set.backgroundColor}"></span>${this.escape(set.label || `Series ${index + 1}`)}`;
      item.addEventListener("click", () => {
        if (this.hidden.has(index)) this.hidden.delete(index); else this.hidden.add(index);
        this.draw();
      });
      legend.appendChild(item);
    });
  }

  legendElement() {
    const next = this.canvas.nextElementSibling;
    return next && next.classList.contains("chart-legend") ? next : null;
  }

  ensureTooltip() {
    return chartTooltip.ensure();
  }

  onMove(event) {
    if (!this.points.length) return;
    const rect = this.canvas.getBoundingClientRect();
    const x = event.clientX - rect.left;
    const y = event.clientY - rect.top;
    let nearest = null;
    let distance = Infinity;
    for (const point of this.points) {
      const d = Math.hypot(point.x - x, point.y - y);
      if (d < distance) {
        distance = d;
        nearest = point;
      }
    }
    if (!nearest || distance > 32) {
      this.onLeave();
      return;
    }
    const options = this.config.options || {};
    const format = options.formatValue || ((value) => String(value));
    chartTooltip.show(this, `<strong>${this.escape(nearest.series)}</strong><span>${this.escape(nearest.label)}</span><span>${this.escape(format(nearest.value, nearest.unit, true))}</span>`, event.clientX, event.clientY);
  }

  onLeave() {
    chartTooltip.hide(this);
  }

  escape(value) {
    return String(value ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  }
};
