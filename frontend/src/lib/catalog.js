// The catalog's progress, as the backend reports it through catalog:activity,
// turned into words and a single bar. Shared by the status bar in App and the
// progress shown on the Settings and first-run screens so the two agree.

const PHASE_LABEL = {
  downloading: 'Downloading catalog…',
  extracting: 'Extracting…',
  copying: 'Copying…',
  indexing: 'Rebuilding index…',
  done: 'Done',
  failed: 'Failed',
}

export function catalogActivityTitle(act) {
  if (!act) return ''
  const what = act.kind === 'index' ? 'Rebuilding index' : 'Updating catalog'
  return act.phase === 'failed' ? `${what} — failed` : what
}

export function catalogActivityLabel(act) {
  if (!act) return ''
  if (act.phase === 'failed') return act.error || 'Failed'
  if (act.phase === 'downloading') return `${PHASE_LABEL.downloading} ${act.percent}%`
  return PHASE_LABEL[act.phase] ?? ''
}

// One bar for the whole job. The download is the only phase with real progress
// and the only one that takes long, so it owns most of the bar; the rest are
// fixed marks so the bar still moves through them.
export function catalogActivityPercent(act) {
  if (!act) return 0
  switch (act.phase) {
    case 'downloading': return Math.round(act.percent * 0.7)
    case 'extracting': return 75
    case 'copying': return 82
    case 'indexing': return act.kind === 'index' ? 50 : 92
    case 'done': return 100
    default: return 0
  }
}

// True while the job is still going: a terminal phase leaves the activity on
// screen only as a report.
export function catalogActivityRunning(act) {
  return !!act && act.phase !== 'done' && act.phase !== 'failed'
}
