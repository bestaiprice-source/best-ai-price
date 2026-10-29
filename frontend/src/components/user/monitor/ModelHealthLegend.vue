<template>
  <div class="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-[11px] text-gray-500 dark:text-dark-400">
    <span v-for="entry in MODEL_HEALTH_LEGEND" :key="entry.id" class="inline-flex items-center gap-1.5">
      <span class="inline-block h-2 w-2 rounded-full" :class="entry.dotClass"></span>
      <span>{{ t(entry.labelKey) }}</span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { MODEL_HEALTH_LEGEND } from '@/constants/modelHealth'

/**
 * The OpenLux-style legend: Normal / High latency / Error / Maintenance.
 * Shared by the dashboard health block, the model catalog and the
 * channel-status screen so the wording cannot drift between them.
 */
const { t } = useI18n()
</script>
