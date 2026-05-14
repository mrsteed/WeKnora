import test from 'node:test';
import assert from 'node:assert/strict';

import { upsertThinkingEvent } from '../utils/thinkingEvent.js';

function createMessage() {
  return {
    agentEventStream: [],
    _eventMap: new Map()
  };
}

test('creates and completes a thinking event when the first chunk is done with content', () => {
  const message = createMessage();
  const now = 1715493600000;

  const event = upsertThinkingEvent(message, {
    content: '# 文档标题\n## 第一章',
    done: true,
    data: {
      event_id: 'document-outline-1',
      synthetic: true,
      stage: 'planning'
    }
  }, now);

  assert.ok(event);
  assert.equal(message.agentEventStream.length, 1);
  assert.equal(event.event_id, 'document-outline-1');
  assert.equal(event.content, '# 文档标题\n## 第一章');
  assert.equal(event.done, true);
  assert.equal(event.thinking, false);
  assert.equal(event.synthetic, true);
  assert.equal(event.stage, 'planning');
  assert.equal(event.duration_ms, 0);
  assert.equal(event.completed_at, now);
});

test('accumulates streaming thinking content across multiple chunks', () => {
  const message = createMessage();
  const start = 1715493600000;

  upsertThinkingEvent(message, {
    content: '正在生成大纲：已识别标题。',
    done: false,
    data: {
      event_id: 'thought-1',
      synthetic: true,
      stage: 'planning'
    }
  }, start);

  const event = upsertThinkingEvent(message, {
    content: '已识别 7 个章节。',
    done: false,
    data: {
      event_id: 'thought-1',
      synthetic: true,
      stage: 'planning'
    }
  }, start + 200);

  assert.ok(event);
  assert.equal(message.agentEventStream.length, 1);
  assert.equal(event.content, '正在生成大纲：已识别标题。已识别 7 个章节。');
  assert.equal(event.done, false);
  assert.equal(event.thinking, true);
});

test('replaces synthetic progress content when replace flag is true', () => {
  const message = createMessage();
  const start = 1715493600000;

  upsertThinkingEvent(message, {
    content: '正在规划完整文档大纲。',
    done: false,
    data: {
      event_id: 'progress-1',
      synthetic: true,
      stage: 'planning',
      replace: true
    }
  }, start);

  const event = upsertThinkingEvent(message, {
    content: '正在规划完整文档大纲，已等待 8 秒。',
    done: false,
    data: {
      event_id: 'progress-1',
      synthetic: true,
      stage: 'planning',
      replace: true
    }
  }, start + 8000);

  assert.ok(event);
  assert.equal(message.agentEventStream.length, 1);
  assert.equal(event.content, '正在规划完整文档大纲，已等待 8 秒。');
});

test('stores structured outline metadata on the thinking event', () => {
  const message = createMessage();
  const event = upsertThinkingEvent(message, {
    content: '# 智慧运行建设方案\n## 建设目标',
    done: true,
    data: {
      event_id: 'outline-1',
      synthetic: true,
      stage: 'planning',
      outline: {
        title: '智慧运行建设方案',
        sections: ['建设目标', '平台架构', '实施保障']
      }
    }
  }, 1715493600000);

  assert.ok(event);
  assert.deepEqual(event.outline, {
    title: '智慧运行建设方案',
    sections: ['建设目标', '平台架构', '实施保障']
  });
});

test('stores structured outline role metadata on the thinking event', () => {
  const message = createMessage();
  const event = upsertThinkingEvent(message, {
    content: '',
    done: true,
    data: {
      event_id: 'outline-role-1',
      synthetic: true,
      stage: 'planning',
      outline_role: 'generated_plan',
      outline_source: 'model_validated_outline',
      base_outline: {
        title: '基线方案',
        sections: ['第一章 现状']
      },
      planning_outline: {
        title: '本轮补充计划',
        sections: ['第4章 智慧运行系统设计']
      }
    }
  }, 1715493600000);

  assert.ok(event);
  assert.equal(event.outline_role, 'generated_plan');
  assert.equal(event.outline_source, 'model_validated_outline');
  assert.deepEqual(event.base_outline, {
    title: '基线方案',
    sections: ['第一章 现状']
  });
  assert.deepEqual(event.planning_outline, {
    title: '本轮补充计划',
    sections: ['第4章 智慧运行系统设计']
  });
});

test('stores structured section progress metadata on the thinking event', () => {
  const message = createMessage();
  const event = upsertThinkingEvent(message, {
    content: '正在检索第 7/8 章“AR眼镜智能作业系统”的本地证据（2/3）：AR 智能作业',
    done: false,
    data: {
      event_id: 'progress-structured-1',
      synthetic: true,
      stage: 'retrieving',
      section_current: 7,
      section_total: 8,
      section_title: 'AR眼镜智能作业系统',
      query_current: 2,
      query_total: 3,
      progress_label: '第 7/8 章：AR眼镜智能作业系统 · 检索 2/3'
    }
  }, 1715493600000);

  assert.ok(event);
  assert.equal(event.section_current, 7);
  assert.equal(event.section_total, 8);
  assert.equal(event.section_title, 'AR眼镜智能作业系统');
  assert.equal(event.query_current, 2);
  assert.equal(event.query_total, 3);
  assert.equal(event.progress_label, '第 7/8 章：AR眼镜智能作业系统 · 检索 2/3');
});

test('keeps previous progress content when backend closes synthetic thought with empty replace', () => {
  const message = createMessage();
  const start = 1715493600000;

  upsertThinkingEvent(message, {
    content: '正在生成第 7/8 章：AR眼镜智能作业系统',
    done: false,
    data: {
      event_id: 'progress-close-1',
      synthetic: true,
      stage: 'generating',
      replace: true,
      section_current: 7,
      section_total: 8,
      section_title: 'AR眼镜智能作业系统'
    }
  }, start);

  const event = upsertThinkingEvent(message, {
    content: '',
    done: true,
    data: {
      event_id: 'progress-close-1',
      synthetic: true,
      replace: true
    }
  }, start + 1000);

  assert.ok(event);
  assert.equal(event.done, true);
  assert.equal(event.content, '正在生成第 7/8 章：AR眼镜智能作业系统');
  assert.equal(event.section_title, 'AR眼镜智能作业系统');
});
