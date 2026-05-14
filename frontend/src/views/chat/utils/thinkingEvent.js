export function upsertThinkingEvent(message, payload, now = Date.now()) {
  if (!message || !payload || typeof payload !== 'object') {
    return null;
  }

  const eventId = payload.data?.event_id;
  if (!eventId) {
    return null;
  }

  if (!message.agentEventStream) {
    message.agentEventStream = [];
  }
  if (!message._eventMap) {
    message._eventMap = new Map();
  }

  let thinkingEvent = message._eventMap.get(eventId);
  const incomingContent = typeof payload.content === 'string' ? payload.content : '';
  const shouldCreate = !thinkingEvent && (incomingContent.length > 0 || payload.done);
  const payloadData = payload.data && typeof payload.data === 'object' ? payload.data : {};

  if (shouldCreate) {
    thinkingEvent = {
      type: 'thinking',
      event_id: eventId,
      content: '',
      done: false,
      startTime: now,
      thinking: !payload.done,
      synthetic: !!payloadData.synthetic,
      stage: typeof payloadData.stage === 'string' ? payloadData.stage : '',
      outline: payloadData.outline && typeof payloadData.outline === 'object' ? payloadData.outline : null,
      outline_role: typeof payloadData.outline_role === 'string' ? payloadData.outline_role : '',
      outline_source: typeof payloadData.outline_source === 'string' ? payloadData.outline_source : '',
      base_outline: payloadData.base_outline && typeof payloadData.base_outline === 'object' ? payloadData.base_outline : null,
      planning_outline: payloadData.planning_outline && typeof payloadData.planning_outline === 'object' ? payloadData.planning_outline : null,
      section_current: Number.isFinite(payloadData.section_current) ? payloadData.section_current : 0,
      section_total: Number.isFinite(payloadData.section_total) ? payloadData.section_total : 0,
      section_title: typeof payloadData.section_title === 'string' ? payloadData.section_title : '',
      query_current: Number.isFinite(payloadData.query_current) ? payloadData.query_current : 0,
      query_total: Number.isFinite(payloadData.query_total) ? payloadData.query_total : 0,
      progress_label: typeof payloadData.progress_label === 'string' ? payloadData.progress_label : ''
    };
    message.agentEventStream.push(thinkingEvent);
    message._eventMap.set(eventId, thinkingEvent);
  }

  if (!thinkingEvent) {
    return null;
  }

  if (payloadData.synthetic !== undefined) {
    thinkingEvent.synthetic = !!payloadData.synthetic;
  }
  if (typeof payloadData.stage === 'string' && payloadData.stage.trim()) {
    thinkingEvent.stage = payloadData.stage;
  }
  if (payloadData.outline && typeof payloadData.outline === 'object') {
    thinkingEvent.outline = payloadData.outline;
  }
  if (typeof payloadData.outline_role === 'string') {
    thinkingEvent.outline_role = payloadData.outline_role;
  }
  if (typeof payloadData.outline_source === 'string') {
    thinkingEvent.outline_source = payloadData.outline_source;
  }
  if (payloadData.base_outline && typeof payloadData.base_outline === 'object') {
    thinkingEvent.base_outline = payloadData.base_outline;
  }
  if (payloadData.planning_outline && typeof payloadData.planning_outline === 'object') {
    thinkingEvent.planning_outline = payloadData.planning_outline;
  }
  if (Number.isFinite(payloadData.section_current) && payloadData.section_current > 0) {
    thinkingEvent.section_current = payloadData.section_current;
  }
  if (Number.isFinite(payloadData.section_total) && payloadData.section_total > 0) {
    thinkingEvent.section_total = payloadData.section_total;
  }
  if (typeof payloadData.section_title === 'string' && payloadData.section_title.trim()) {
    thinkingEvent.section_title = payloadData.section_title;
  }
  if (Number.isFinite(payloadData.query_current) && payloadData.query_current > 0) {
    thinkingEvent.query_current = payloadData.query_current;
  }
  if (Number.isFinite(payloadData.query_total) && payloadData.query_total > 0) {
    thinkingEvent.query_total = payloadData.query_total;
  }
  if (typeof payloadData.progress_label === 'string' && payloadData.progress_label.trim()) {
    thinkingEvent.progress_label = payloadData.progress_label;
  }

  if (incomingContent || payloadData.replace) {
    if (payloadData.replace) {
      if (!incomingContent) {
        // Preserve the last non-empty progress snapshot when the backend closes
        // the synthetic thought with an empty replace event.
      } else {
        thinkingEvent.content = incomingContent;
      }
    } else {
      thinkingEvent.content += incomingContent;
    }
  }

  if (payload.done) {
    thinkingEvent.done = true;
    thinkingEvent.thinking = false;
    thinkingEvent.duration_ms = payloadData.duration_ms || (now - (thinkingEvent.startTime || now));
    thinkingEvent.completed_at = payloadData.completed_at || now;
  }

  return thinkingEvent;
}
