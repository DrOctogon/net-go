<!--
  StubDetectionsCard.svelte - Test double for DetectionsCard.svelte

  Purpose: Let DetectionsPage.test.ts drive fetchDetections() races without
  mounting the real DetectionsCard/DetectionsList tree (weather, thumbnails,
  pagination UI, etc.), which is out of scope for a stale-response race test.
-->
<script lang="ts">
  import type { DetectionsListData, DetectionSortBy } from '$lib/types/detection.types';

  interface Props {
    data: DetectionsListData | null;
    loading?: boolean;
    error?: string | null;
    onPageChange: (_newPage: number) => void;
    onDetailsClick: (_id: number) => void;
    onRefresh: () => void;
    onNumResultsChange: (_numResults: number) => void;
    onSortChange?: (_sortBy: DetectionSortBy) => void;
  }

  let { data = null, loading = false, onSortChange }: Props = $props();
</script>

<div data-testid="loading-state">{loading}</div>
<div data-testid="total-results">{data?.totalResults ?? 0}</div>
<button type="button" data-testid="trigger-sort-a" onclick={() => onSortChange?.('date_asc')}>
  sort-a
</button>
<button
  type="button"
  data-testid="trigger-sort-b"
  onclick={() => onSortChange?.('confidence_desc')}
>
  sort-b
</button>
