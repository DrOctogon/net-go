import { fetchWithCSRF } from './api';
import { t } from '$lib/i18n';
import { toastActions } from '$lib/stores/toast';
import { loggers } from './logger';

const logger = loggers.ui;

/**
 * HTTP 503 from the cluster-management endpoints is not a failure: it means
 * voice-print clustering is switched off, so there are no clusters to change.
 * Worth its own message so users stop retrying a disabled feature.
 */
const CLUSTERING_UNAVAILABLE_STATUS = 503;

/**
 * True when the rejection carries HTTP 503. Duck-typed on `status` rather than
 * `instanceof ApiError` so test doubles of the api module work unchanged.
 */
function isClusteringUnavailable(error: unknown): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    (error as { status?: unknown }).status === CLUSTERING_UNAVAILABLE_STATUS
  );
}

/** Toast the failure, preferring the "voice-print analysis is off" wording on 503. */
function reportFailure(error: unknown, fallbackMessage: string, logMessage: string): void {
  toastActions.error(
    isClusteringUnavailable(error) ? t('detections.speaker.clusteringOff') : fallbackMessage
  );
  logger.error(logMessage, error);
}

/**
 * Fold `sourceId` into `targetId`: the target survives and keeps its own name
 * (an unnamed target adopts the source's), the source's detections are
 * relabelled, and the source id is retired. Toasts the outcome.
 *
 * Returns `true` on success; callers refetch the roster and any cluster-derived
 * lists themselves.
 */
export async function mergeSpeakerClusters(targetId: string, sourceId: string): Promise<boolean> {
  try {
    await fetchWithCSRF(`/api/v2/speakers/${targetId}/merge`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ sourceId }),
    });
    toastActions.success(t('detections.speaker.merge.success'));
    return true;
  } catch (error) {
    reportFailure(error, t('detections.speaker.merge.failed'), 'Failed to merge speakers:');
    return false;
  }
}

/**
 * Forget one voice-print cluster: its name row is deleted and its detections
 * are unlabelled (clips and history are kept). The id is never reissued.
 * Toasts the outcome.
 *
 * Returns `true` on success; callers refetch the roster themselves.
 */
export async function forgetSpeakerCluster(speakerId: string): Promise<boolean> {
  try {
    await fetchWithCSRF(`/api/v2/speakers/${speakerId}`, { method: 'DELETE' });
    toastActions.success(t('settings.speakers.forget.success'));
    return true;
  } catch (error) {
    reportFailure(error, t('settings.speakers.forget.failed'), 'Failed to forget speaker:');
    return false;
  }
}
