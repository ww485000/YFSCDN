<script setup lang="ts">
// Lightweight SVG line/area chart (zero dependencies).
// props: series of {label, values:number[]} with one line per series.
interface Series {
  name: string;
  color: string;
  values: number[];
}
const props = defineProps<{
  labels: string[];
  series: Series[];
  height?: number;
  unit?: (n: number) => string;
}>();

const W = 720;
const H = props.height ?? 240;
const PAD_L = 56;
const PAD_R = 16;
const PAD_T = 16;
const PAD_B = 28;

const fmt = (n: number): string => (props.unit ? props.unit(n) : String(n));

function maxOf(): number {
  let m = 0;
  for (const s of props.series) for (const v of s.values) if (v > m) m = v;
  return m || 1;
}

function x(i: number): number {
  const n = props.labels.length;
  if (n <= 1) return PAD_L + (W - PAD_L - PAD_R) / 2;
  return PAD_L + (i * (W - PAD_L - PAD_R)) / (n - 1);
}
function y(v: number): number {
  return PAD_T + (H - PAD_T - PAD_B) * (1 - v / maxOf());
}
function path(values: number[]): string {
  return values.map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ');
}
function area(values: number[]): string {
  const base = H - PAD_B;
  return `${path(values)} L${x(values.length - 1).toFixed(1)},${base} L${x(0).toFixed(1)},${base} Z`;
}

// 4 tick values for the y axis
const ticks = ((): number[] => {
  const m = maxOf();
  return [0, 0.25, 0.5, 0.75, 1].map((f) => Math.round(m * f));
})();

// x labels: show at most 8
const xStep = Math.max(1, Math.ceil(props.labels.length / 8));
</script>

<template>
  <svg :viewBox="`0 0 ${W} ${H}`" class="w-full" style="max-width: 760px">
    <!-- grid + y ticks -->
    <g v-for="(t, i) in ticks" :key="i">
      <line :x1="PAD_L" :x2="W - PAD_R" :y1="y(t)" :y2="y(t)" stroke="#f0f0f0" stroke-width="1" />
      <text :x="PAD_L - 6" :y="y(t) + 4" text-anchor="end" font-size="10" fill="#999">{{ fmt(t) }}</text>
    </g>
    <!-- x labels -->
    <text
      v-for="(lb, i) in labels"
      :key="i"
      v-show="i % xStep === 0"
      :x="x(i)"
      :y="H - 8"
      text-anchor="middle"
      font-size="10"
      fill="#999"
    >{{ lb }}</text>
    <!-- series -->
    <g v-for="s in series" :key="s.name">
      <path v-if="series.indexOf(s) === 0 && s.values.length > 1" :d="area(s.values)" :fill="s.color" opacity="0.08" />
      <path :d="path(s.values)" fill="none" :stroke="s.color" stroke-width="2" />
      <circle v-for="(v, i) in s.values" :key="i" :cx="x(i)" :cy="y(v)" r="2.5" :fill="s.color">
        <title>{{ `${labels[i]} · ${s.name}: ${fmt(v)}` }}</title>
      </circle>
    </g>
    <!-- legend -->
    <g v-for="(s, i) in series" :key="'lg' + s.name">
      <rect :x="PAD_L + i * 110" :y="4" width="12" height="3" :fill="s.color" />
      <text :x="PAD_L + i * 110 + 16" y="9" font-size="10" fill="#666">{{ s.name }}</text>
    </g>
  </svg>
</template>
