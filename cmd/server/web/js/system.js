/**
 * system.js - System Information
 */
function renderSystem() {
    var content = document.getElementById('content');
    content.innerHTML = '<h2>ℹ️ 系统信息</h2>' +
        '<div class="card" style="margin-bottom:16px" id="sysStatus"></div>' +
        '<div style="display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-bottom:16px">' +
        '<div class="card" id="sysStorage"></div><div class="card" id="sysEmbedding"></div></div>' +
        '<div style="display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-bottom:16px">' +
        '<div class="card" id="sysDedup"></div><div class="card" id="sysTtl"></div></div>' +
        '<div class="card" id="sysScoring" style="margin-bottom:16px"></div>' +
        '<div class="card" id="sysGovernance"></div>';

    api.systemConfig().then(function(cfg) {
        document.getElementById('sysStatus').innerHTML = '<div class="card-header"><h2>服务状态</h2></div>' +
            '<table class="config-table"><tr><td class="config-label">状态</td><td style="color:var(--active)">✅ Running</td></tr>' +
            '<tr><td class="config-label">版本</td><td>' + cfg.version + '</td></tr>' +
            '<tr><td class="config-label">运行时间</td><td>' + formatDuration(cfg.uptime_seconds) + '</td></tr></table>';

        var s = cfg.storage || {};
        document.getElementById('sysStorage').innerHTML = '<div class="card-header"><h2>存储配置</h2></div>' +
            '<table class="config-table"><tr><td class="config-label">数据库</td><td>SQLite</td></tr>' +
            '<tr><td class="config-label">路径</td><td style="font-family:monospace;font-size:12px">' + escHtml(s.sqlite_path) + '</td></tr>' +
            '<tr><td class="config-label">文件大小</td><td>' + formatBytes(s.db_size_bytes) + '</td></tr>' +
            '<tr><td class="config-label">向量存储</td><td>' + escHtml(s.vector_provider) + '</td></tr></table>';

        var e = cfg.embedding || {};
        document.getElementById('sysEmbedding').innerHTML = '<div class="card-header"><h2>Embedding</h2></div>' +
            '<table class="config-table"><tr><td class="config-label">提供商</td><td>' + escHtml(e.provider) + '</td></tr>' +
            '<tr><td class="config-label">模型</td><td>' + escHtml(e.model) + '</td></tr>' +
            '<tr><td class="config-label">维度</td><td>' + e.dimensions + '</td></tr></table>';

        var d = cfg.dedup_thresholds || {};
        document.getElementById('sysDedup').innerHTML = '<div class="card-header"><h2>去重阈值</h2></div>' +
            '<table class="config-table">' +
            buildThresholdRow('identity', d.identity || 0.95, 'var(--identity)') +
            buildThresholdRow('principle', d.principle || 0.90, 'var(--principle)') +
            buildThresholdRow('knowledge', d.knowledge || 0.85, 'var(--knowledge)') +
            buildThresholdRow('working', d.working || 0.70, 'var(--working)') +
            '</table>';

        var t = cfg.ttl || {};
        document.getElementById('sysTtl').innerHTML = '<div class="card-header"><h2>TTL策略</h2></div>' +
            '<table class="config-table"><tr><td class="config-label">扫描间隔</td><td>' + (t.scan_interval_hours || 6) + 'h</td></tr>' +
            '<tr><td class="config-label">降级倍数</td><td>' + (t.degrade_multiplier || 2) + 'x TTL</td></tr>' +
            '<tr><td class="config-label">归档倍数</td><td>' + (t.archive_multiplier || 3) + 'x TTL</td></tr></table>';

        var sc = cfg.scoring || {};
        document.getElementById('sysScoring').innerHTML = '<div class="card-header"><h2>评分权重</h2></div>' +
            '<div style="display:flex;gap:20px;flex-wrap:wrap">' +
            buildWeightBlock('相似度', sc.similarity_weight || 0.40, 'var(--accent)') +
            buildWeightBlock('优先级', sc.priority_weight || 0.25, 'var(--principle)') +
            buildWeightBlock('热度', sc.access_count_weight || 0.15, 'var(--working)') +
            buildWeightBlock('分类', sc.category_weight || 0.10, 'var(--knowledge)') +
            buildWeightBlock('紧急度', sc.urgency_weight || 0.10, 'var(--degraded)') +
            '</div>';

        var g = cfg.governance || {};
        document.getElementById('sysGovernance').innerHTML = '<div class="card-header"><h2>治理参数</h2></div>' +
            '<table class="config-table"><tr><td class="config-label">压缩阈值</td><td>' + (g.compress_threshold || 0.85) + '</td></tr>' +
            '<tr><td class="config-label">最大记忆数/Agent</td><td>' + (g.max_memories_per_agent || 10000) + '</td></tr>' +
            '<tr><td class="config-label">最大内容长度</td><td>' + (g.max_content_length || 10000) + '</td></tr></table>';
    }).catch(function(e) { showToast('加载系统信息失败: ' + e.message, 'error'); });
}

function buildThresholdRow(name, value, color) {
    var pct = Math.round(value * 100);
    return '<tr><td class="config-label">' + name + '</td><td><div class="progress-bar"><div class="progress-fill" style="width:' + pct + '%;background:' + color + '"></div></div> ' + pct + '%</td></tr>';
}

function buildWeightBlock(label, value, color) {
    var pct = Math.round(value * 100);
    return '<div style="text-align:center;min-width:80px"><div style="font-size:24px;font-weight:700;color:' + color + '">' + pct + '%</div><div style="font-size:12px;color:var(--text-dim)">' + label + '</div></div>';
}
