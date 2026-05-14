import test from 'node:test';
import assert from 'node:assert/strict';

import {
  extractStructuredPlanningOutlineFromText,
  extractPlanningOutlineFromCompleteEvent,
  extractPlanningOutlineFromText,
  getPlanningOutlineFromEvent,
  getPlanningOutlineFromThinkingEvent,
  pickStructuredPlanningOutline,
  shouldAllowPlanningOutlineArtifactFallback,
} from '../utils/planningOutline.js';

test('prefers structured outline metadata over markdown fallback', () => {
  const outline = getPlanningOutlineFromEvent({
    type: 'thinking',
    outline: {
      title: '北海电厂二期智慧电厂项目投标技术方案',
      sections: ['项目背景与建设目标', '数据湖与基础算力平台技术方案']
    }
  }, '# 错误标题\n## 错误章节');

  assert.deepEqual(outline, {
    title: '北海电厂二期智慧电厂项目投标技术方案',
    sections: ['项目背景与建设目标', '数据湖与基础算力平台技术方案'],
    outlineOnly: true
  });
});

test('reads structured outline from thinking event metadata payload', () => {
  const outline = getPlanningOutlineFromEvent({
    type: 'thinking',
    data: {
      outline: {
        title: '智慧运行建设方案',
        sections: ['建设目标', '平台架构', '实施保障']
      }
    }
  }, '');

  assert.deepEqual(outline, {
    title: '智慧运行建设方案',
    sections: ['建设目标', '平台架构', '实施保障'],
    outlineOnly: true
  });
});

test('extracts complete-event outline from structured payload', () => {
  const outline = extractPlanningOutlineFromCompleteEvent({
    type: 'agent_complete',
    outline: {
      title: '智慧运行建设方案',
      sections: ['建设目标', '平台架构', '实施保障']
    }
  });

  assert.deepEqual(outline, {
    title: '智慧运行建设方案',
    sections: ['建设目标', '平台架构', '实施保障'],
    outlineOnly: true
  });
});

test('ignores base document outline in complete event payload', () => {
  const outline = extractPlanningOutlineFromCompleteEvent({
    type: 'agent_complete',
    outline_role: 'base_document',
    outline: {
      title: '基线文档结构',
      sections: ['第1章 现状', '第2章 架构']
    }
  });

  assert.equal(outline, null);
});

test('prefers planning outline over base document outline metadata', () => {
  const outline = pickStructuredPlanningOutline({
    type: 'agent_complete',
    outline_role: 'continuation_plan',
    outline: {
      title: '基线文档结构',
      sections: ['第1章 现状', '第2章 架构']
    },
    planning_outline: {
      title: '本轮补充计划',
      sections: ['第4章 智慧运行系统设计']
    }
  });

  assert.deepEqual(outline, {
    title: '本轮补充计划',
    sections: ['第4章 智慧运行系统设计']
  });
});

test('disables artifact fallback when complete event is marked as base document', () => {
  const allowed = shouldAllowPlanningOutlineArtifactFallback({
    completeEvent: {
      type: 'agent_complete',
      outline_role: 'base_document'
    }
  });

  assert.equal(allowed, false);
});

test('disables artifact fallback for document edit event streams', () => {
  const allowed = shouldAllowPlanningOutlineArtifactFallback({
    eventStream: [
      {
        type: 'thinking',
        synthetic: true,
        stage: 'document_edit',
        content: '正在分析基线文档并生成修订补丁，请稍候。'
      }
    ]
  });

  assert.equal(allowed, false);
});

test('keeps artifact fallback enabled for ordinary planning streams', () => {
  const allowed = shouldAllowPlanningOutlineArtifactFallback({
    eventStream: [
      {
        type: 'thinking',
        synthetic: true,
        stage: 'planning',
        outline_role: 'generated_plan',
        content: '正在生成文档规划。'
      }
    ]
  });

  assert.equal(allowed, true);
});

test('falls back to markdown parsing when metadata outline is absent', () => {
  const outline = getPlanningOutlineFromEvent({
    type: 'thinking'
  }, '# 智慧运行建设方案\n## 建设目标\n## 平台架构');

  assert.deepEqual(outline, {
    title: '智慧运行建设方案',
    sections: ['建设目标', '平台架构'],
    outlineOnly: true
  });
});

test('thinking outline detection ignores freeform non-synthetic reasoning markdown', () => {
  const outline = getPlanningOutlineFromThinkingEvent({
    type: 'thinking',
    synthetic: false,
    stage: 'generating'
  }, '# 智慧运行建设方案\n## 建设目标\n## 平台架构');

  assert.equal(outline, null);
});

test('thinking outline detection allows markdown fallback for synthetic planning events', () => {
  const outline = getPlanningOutlineFromThinkingEvent({
    type: 'thinking',
    synthetic: true,
    stage: 'planning'
  }, '# 智慧运行建设方案\n## 建设目标\n## 平台架构');

  assert.deepEqual(outline, {
    title: '智慧运行建设方案',
    sections: ['建设目标', '平台架构'],
    outlineOnly: true
  });
});

test('normalizes concatenated markdown headings into an outline preview', () => {
  const outline = extractPlanningOutlineFromText('# 智慧运行建设方案##建设目标##平台架构##实施保障');

  assert.deepEqual(outline, {
    title: '智慧运行建设方案',
    sections: ['建设目标', '平台架构', '实施保障'],
    outlineOnly: true
  });
});

test('extracts structured outline payload from rendered markdown', () => {
  const outline = extractStructuredPlanningOutlineFromText('# 智慧运行建设方案\n## 建设目标\n## 平台架构');

  assert.deepEqual(outline, {
    title: '智慧运行建设方案',
    sections: ['建设目标', '平台架构']
  });
});

test('keeps structured outline metadata when merged events are synthesized', () => {
  const outline = pickStructuredPlanningOutline(
    {
      type: 'thinking',
      outline: {
        title: '智慧运行建设方案',
        sections: ['建设目标', '平台架构', '实施保障']
      }
    },
    {
      type: 'thinking',
      content: '正在生成大纲：已识别 3 个章节。'
    }
  );

  assert.deepEqual(outline, {
    title: '智慧运行建设方案',
    sections: ['建设目标', '平台架构', '实施保障']
  });
});
