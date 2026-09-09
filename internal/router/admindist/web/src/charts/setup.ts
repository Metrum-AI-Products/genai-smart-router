// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

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
import "@/charts/utcDateAdapter";

ChartJS.register(CategoryScale, LinearScale, TimeScale, BarElement, LineElement, PointElement, Tooltip, Legend, Filler);
