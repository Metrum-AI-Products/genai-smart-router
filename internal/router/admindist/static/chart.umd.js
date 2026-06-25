window.Chart = class {
  constructor(canvas, config) {
    this.canvas = canvas;
    this.config = config || {};
    this.draw();
  }
  destroy() {}
  draw() {
    const ctx = this.canvas.getContext("2d");
    const width = this.canvas.clientWidth || 320;
    const height = this.canvas.clientHeight || 180;
    this.canvas.width = width * devicePixelRatio;
    this.canvas.height = height * devicePixelRatio;
    ctx.scale(devicePixelRatio, devicePixelRatio);
    ctx.clearRect(0, 0, width, height);
    const datasets = (this.config.data && this.config.data.datasets) || [];
    const values = datasets.flatMap((set) => set.data || []).map(Number);
    const max = Math.max(1, ...values);
    ctx.strokeStyle = "#c7cdd5";
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(32, 12);
    ctx.lineTo(32, height - 24);
    ctx.lineTo(width - 8, height - 24);
    ctx.stroke();
    datasets.forEach((set, setIndex) => {
      const data = (set.data || []).map(Number);
      const color = set.borderColor || set.backgroundColor || ["#1f6feb", "#d97706", "#059669"][setIndex % 3];
      ctx.strokeStyle = color;
      ctx.fillStyle = color;
      ctx.lineWidth = 2;
      ctx.beginPath();
      data.forEach((value, index) => {
        const x = 32 + (index * Math.max(1, width - 48)) / Math.max(1, data.length - 1);
        const y = height - 24 - (value / max) * Math.max(1, height - 40);
        if (index === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
      });
      ctx.stroke();
      data.forEach((value, index) => {
        const x = 32 + (index * Math.max(1, width - 48)) / Math.max(1, data.length - 1);
        const y = height - 24 - (value / max) * Math.max(1, height - 40);
        ctx.beginPath();
        ctx.arc(x, y, 2.5, 0, Math.PI * 2);
        ctx.fill();
      });
    });
  }
};
