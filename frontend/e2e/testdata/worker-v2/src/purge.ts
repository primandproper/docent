import { setTimeout as sleep } from 'node:timers/promises';
import { cache } from './cache';
import { log } from './log';
import type { Invalidate } from './messages';

const MAX_ATTEMPTS = 5;

/** Evicts every cache entry for an item, retrying with backoff. */
export async function purge(message: Invalidate): Promise<void> {
  for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
    try {
      await cache.purge(`/items/${message.id}`);
      return;
    } catch (err) {
      log.warn('purge failed', { attempt, id: message.id });
      if (attempt === MAX_ATTEMPTS) {
        throw err;
      }
      await sleep(2 ** attempt * 100);
    }
  }
}
