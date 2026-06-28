import {
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  Filler,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  TimeScale,
  Tooltip,
} from "chart.js";

ChartJS.register(CategoryScale, LinearScale, TimeScale, BarElement, LineElement, PointElement, Tooltip, Legend, Filler);
