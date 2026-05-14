export function normalizePlanningOutlineMarkdown(content) {
  if (!content || typeof content !== 'string') {
    return '';
  }

  return content
    .replace(/\r\n?/g, '\n')
    .replace(/([^\n])##\s*/g, '$1\n## ')
    .replace(/^##(?=\S)/gm, '## ')
    .replace(/^#(?!#)(?=\S)/gm, '# ')
    .trim();
}

export function normalizePlanningOutlineSections(sections) {
  if (!Array.isArray(sections)) {
    return [];
  }

  const seen = new Set();
  const normalized = [];
  for (const rawSection of sections) {
    let section = '';
    if (typeof rawSection === 'string') {
      section = rawSection.trim();
    } else if (rawSection && typeof rawSection === 'object') {
      if (typeof rawSection.heading === 'string') {
        section = rawSection.heading.trim();
      } else if (typeof rawSection.title === 'string') {
        section = rawSection.title.trim();
      }
    }
    if (!section || seen.has(section)) {
      continue;
    }
    seen.add(section);
    normalized.push(section);
  }
  return normalized;
}

export function normalizePlanningOutlinePreview(outline) {
  if (!outline || typeof outline !== 'object') {
    return null;
  }

  const title = typeof outline.title === 'string' ? outline.title.trim() : '';
  const sections = normalizePlanningOutlineSections(outline.sections);
  if (!title && sections.length === 0) {
    return null;
  }

  return {
    title,
    sections,
    outlineOnly: Boolean(outline.outlineOnly)
  };
}

export function normalizeStructuredPlanningOutline(rawOutline) {
  if (!rawOutline || typeof rawOutline !== 'object') {
    return null;
  }

  const normalized = normalizePlanningOutlinePreview({
    title: rawOutline.title,
    sections: rawOutline.sections,
    outlineOnly: true
  });
  if (!normalized) {
    return null;
  }

  return {
    title: normalized.title,
    sections: normalized.sections
  };
}

export function extractPlanningOutlineFromText(content) {
  const normalized = normalizePlanningOutlineMarkdown(content);
  if (!normalized) {
    return null;
  }

  const lines = normalized
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean);
  if (lines.length === 0) {
    return null;
  }

  let title = '';
  const sections = [];
  let nonHeadingLines = 0;

  for (const line of lines) {
    if (line.startsWith('# ') && !line.startsWith('## ')) {
      if (!title) {
        title = line.slice(2).trim();
        continue;
      }
      nonHeadingLines += 1;
      continue;
    }
    if (line.startsWith('## ')) {
      const section = line.slice(3).trim();
      if (section) {
        sections.push(section);
        continue;
      }
    }
    nonHeadingLines += 1;
  }

  if (!title && sections.length === 0) {
    return null;
  }

  return {
    title,
    sections,
    outlineOnly: nonHeadingLines === 0
  };
}

export function extractStructuredPlanningOutlineFromText(content) {
  const outline = extractPlanningOutlineFromText(content);
  if (!outline) {
    return null;
  }

  return {
    title: outline.title,
    sections: outline.sections
  };
}

export function extractPlanningOutlineFromStructured(rawOutline) {
  const normalized = normalizeStructuredPlanningOutline(rawOutline);
  if (!normalized) {
    return null;
  }

  return normalizePlanningOutlinePreview({
    title: normalized.title,
    sections: normalized.sections,
    outlineOnly: true
  });
}

function trimOutlineRole(value) {
  return typeof value === 'string' ? value.trim() : '';
}

function pickStructuredPlanningOutlineFromCandidate(candidate) {
  if (!candidate || typeof candidate !== 'object') {
    return null;
  }

  const role = trimOutlineRole(candidate?.outline_role || candidate?.data?.outline_role);
  const planningOutline = normalizeStructuredPlanningOutline(candidate?.planning_outline || candidate?.data?.planning_outline);
  if (planningOutline) {
    return planningOutline;
  }

  if (role === 'base_document') {
    return null;
  }

  return normalizeStructuredPlanningOutline(candidate?.outline || candidate?.data?.outline || candidate);
}

export function pickStructuredPlanningOutline(...candidates) {
  for (const candidate of candidates) {
    const normalized = pickStructuredPlanningOutlineFromCandidate(candidate);
    if (normalized) {
      return normalized;
    }
  }

  return null;
}

export function extractPlanningOutlineFromCompleteEvent(event) {
  if (!event || typeof event !== 'object') {
    return null;
  }

  return extractPlanningOutlineFromStructured(pickStructuredPlanningOutline(event));
}

export function shouldAllowPlanningOutlineArtifactFallback({ completeEvent = null, eventStream = null } = {}) {
  const stream = Array.isArray(eventStream) ? eventStream : [];
  const hasBaseDocumentRole = trimOutlineRole(completeEvent?.outline_role) === 'base_document' ||
    stream.some((event) => trimOutlineRole(event?.outline_role || event?.data?.outline_role) === 'base_document');
  if (hasBaseDocumentRole) {
    return false;
  }

  const hasDocumentEditStage = stream.some((event) => {
    const stage = typeof event?.stage === 'string'
      ? event.stage.trim()
      : typeof event?.data?.stage === 'string'
        ? event.data.stage.trim()
        : '';
    return stage === 'document_edit';
  });
  if (hasDocumentEditStage) {
    return false;
  }

  return true;
}

export function getPlanningOutlineFromEvent(event, fallbackContent = '') {
  if (!event || typeof event !== 'object') {
    return null;
  }

  const structuredOutline = extractPlanningOutlineFromStructured(pickStructuredPlanningOutline(event));
  if (structuredOutline) {
    return structuredOutline;
  }

  return extractPlanningOutlineFromText(fallbackContent);
}

export function getPlanningOutlineFromThinkingEvent(event, fallbackContent = '') {
  if (!event || typeof event !== 'object') {
    return null;
  }

  const structuredOutline = extractPlanningOutlineFromStructured(pickStructuredPlanningOutline(event));
  if (structuredOutline) {
    return structuredOutline;
  }

  const stage = typeof event.stage === 'string' ? event.stage.trim() : '';
  const isSyntheticPlanning = event.synthetic === true && stage === 'planning';
  if (!isSyntheticPlanning) {
    return null;
  }

  return extractPlanningOutlineFromText(fallbackContent);
}
