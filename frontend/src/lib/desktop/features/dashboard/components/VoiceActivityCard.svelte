<!--
VoiceActivityCard.svelte — Dashboard widget: voice-detection totals per hour for today.

Purpose:
- Fetches GET /api/v2/analytics/time/distribution/hourly for today (no species filter)
  and renders a responsive 24-bar D3 bar chart showing total voice detections per hour.
- Mirrors the dashboard card pattern of CurrentlyHearingCard / DailySummaryCard
  (section container, header with title+subtitle, loading / error / empty states).

Chart:
- D3 scaleBand (x = hour 0–23) + scaleLinear (y = count)
- Responsive width via bind:clientWidth on the chart container div
- Bars filled with var(--color-primary) — dark-mode aware
- Hour labels at 0, 6, 12, 18
- Accessible: role="img" + aria-label on svg, <title> per bar

Props: none (self-contained; fetches its own data on mount)
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { scaleBand, scaleLinear } from 'd3-scale';
  import { max } from 'd3-array';
  import { BarChart2 } from '@lucide/svelte';
  import { t } from '$lib/i18n';
  import { api } from '$lib/utils/api';
  import { getLocalDateString } from '$lib/utils/date';
  import { getLogger } from '$lib/utils/logger';

  const logger = getLogger('dashboard');

  // Shape of each entry returned by the hourly distribution endpoint
  interface HourlyData {
    hour: number;
    count: number;
  }

  // Pre-computed bar geometry consumed by the SVG template
  interface BarDatum {
    x: number;
    y: number;
    w: number;
    h: number;
    count: number;
    hour: number;
  }

  // ── Reactive state ────────────────────────────────────────────────────────

  /** Width of the chart container element (updated by Svelte's bind:clientWidth). */
  let containerWidth = $state(0);

  let data = $state<HourlyData[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // ── Chart layout constants ────────────────────────────────────────────────

  const MARGIN = { top: 12, right: 8, bottom: 24, left: 28 };
  const CHART_HEIGHT = 148;

  // ── Derived chart geometry ────────────────────────────────────────────────

  let chartData = $derived.by((): { bars: BarDatum[]; innerW: number; innerH: number } => {
    const innerW = Math.max(0, containerWidth - MARGIN.left - MARGIN.right);
    const innerH = CHART_HEIGHT - MARGIN.top - MARGIN.bottom;

    if (data.length === 0 || innerW <= 0) {
      return { bars: [], innerW, innerH };
    }

    const xScale = scaleBand<number>()
      .domain(data.map(d => d.hour))
      .range([0, innerW])
      .padding(0.15);

    const maxCount = Math.max(1, max(data, d => d.count) ?? 1);
    const yScale = scaleLinear().domain([0, maxCount]).range([innerH, 0]).nice();

    const bars: BarDatum[] = data.map(d => ({
      x: xScale(d.hour) ?? 0,
      y: yScale(d.count),
      w: xScale.bandwidth(),
      h: Math.max(0, innerH - yScale(d.count)),
      count: d.count,
      hour: d.hour,
    }));

    return { bars, innerW, innerH };
  });

  let totalDetections = $derived(data.reduce((sum, d) => sum + d.count, 0));
  let hasData = $derived(totalDetections > 0);

  // ── Data fetching ─────────────────────────────────────────────────────────

  async function fetchData(): Promise<void> {
    loading = true;
    error = null;
    try {
      const today = getLocalDateString();
      const params = new URLSearchParams({ start_date: today, end_date: today });
      const result = await api.get<HourlyData[]>(
        `/api/v2/analytics/time/distribution/hourly?${params.toString()}`
      );
      data = Array.isArray(result) ? result : [];
    } catch (err) {
      error = err instanceof Error ? err.message : t('dashboard.voiceActivity.errors.load');
      logger.error('VoiceActivityCard: failed to fetch hourly voice data', err);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    void fetchData();
  });
</script>

<section
  class="card col-span-12 flex h-full flex-col rounded-2xl border border-[var(--color-base-200)] bg-[var(--color-base-100)] shadow-sm"
>
  <!-- Card Header -->
  <div class="flex items-center gap-3 border-b border-[var(--color-base-200)] px-6 py-4">
    <BarChart2 class="size-5 shrink-0 text-[var(--color-primary)]" aria-hidden="true" />
    <div class="flex flex-col">
      <h3 class="font-semibold">{t('dashboard.voiceActivity.title')}</h3>
      <p class="text-sm text-[var(--color-base-content)]/60">
        {t('dashboard.voiceActivity.subtitle')}
      </p>
    </div>
  </div>

  <!-- Card Content -->
  <div class="flex flex-1 flex-col px-4 pb-4 pt-3">
    {#if loading}
      <!-- Loading state: spinner + labeled text -->
      <div
        class="flex flex-1 items-center justify-center gap-2 py-6"
        role="status"
        aria-live="polite"
        aria-label={t('dashboard.voiceActivity.loading')}
      >
        <div
          class="h-5 w-5 animate-spin rounded-full border-2 border-[var(--color-primary)] border-t-transparent"
          aria-hidden="true"
        ></div>
        <span class="text-sm text-[var(--color-base-content)]/60">
          {t('dashboard.voiceActivity.loading')}
        </span>
      </div>
    {:else if error}
      <!-- Error state: alert with descriptive message -->
      <div class="flex flex-1 items-center justify-center py-6" role="alert" aria-live="assertive">
        <p
          class="rounded-lg bg-[var(--color-error)]/10 px-4 py-3 text-sm text-[var(--color-error)]"
        >
          {error}
        </p>
      </div>
    {:else if !hasData}
      <!-- Empty state: explains how to populate -->
      <div class="flex flex-1 items-center justify-center py-6">
        <p class="text-sm text-[var(--color-base-content)]/40">
          {t('dashboard.voiceActivity.empty')}
        </p>
      </div>
    {:else}
      <!-- Chart: bind:clientWidth drives responsive width recomputation -->
      <div bind:clientWidth={containerWidth} class="w-full">
        <svg
          width={containerWidth}
          height={CHART_HEIGHT}
          viewBox="0 0 {containerWidth} {CHART_HEIGHT}"
          role="img"
          aria-label={t('dashboard.voiceActivity.chartAriaLabel')}
          class="overflow-visible"
        >
          <g transform="translate({MARGIN.left},{MARGIN.top})">
            {#each chartData.bars as bar (bar.hour)}
              <!-- Bar rectangle -->
              <rect
                x={bar.x}
                y={bar.y}
                width={bar.w}
                height={bar.h}
                fill="var(--color-primary)"
                opacity="0.72"
                rx="2"
              >
                <title
                  >{t('dashboard.voiceActivity.bar.tooltip', {
                    count: bar.count,
                    hour: bar.hour,
                  })}</title
                >
              </rect>

              <!-- Hour label every 6 hours: 0, 6, 12, 18 -->
              {#if bar.hour % 6 === 0}
                <text
                  x={bar.x + bar.w / 2}
                  y={chartData.innerH + 16}
                  text-anchor="middle"
                  fill="currentColor"
                  opacity="0.45"
                  font-size="9"
                >
                  {bar.hour}
                </text>
              {/if}
            {/each}
          </g>
        </svg>
      </div>
    {/if}
  </div>
</section>
