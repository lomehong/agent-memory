/**
 * dashboard.js - Overview Dashboard
 */
var dashCharts = { cat: null, status: null };

function renderDashboard() {
    var content = document.getElementById('content');
    content.innerHTML = '<h2>📊 概览仪表盘</h2>' +
        '<div class="stats-grid">' +
        '<div class="stat-card"><div class="stat-value" id="statTotal">-</div><div class="stat-label">总记忆数</div></div>' +
        '<div class="stat-card"><div class="stat-value" id="statActive" style="color:var(--active)">-</div><div class="stat-label">Active</div></div>' +
        '<div class="stat-card"><div class="stat-value" id="statDegraded" style="color:var(--degraded)">-</div><div class="stat-label">Degraded</div></div>' +
        '<div class="stat-card"><div class="stat-value" id="statArchived" style="color:var(--archived)">-</div><div class="stat-label">Archived</div></div>' +
        '</div>' +
        '<div class="charts-grid" id="chartsArea">' +
        '<div class="card"><div class="card-header"><h2>分类分布</h2></div><div class="chart-container"><canvas id="catChart"></canvas></div></div>' +
        '<div class="card"><div class="card-header"><h2>状态分布</h2></div><div class="chart-container"><canvas id="statusChart"></canvas></div></div>' +
        '</div>' +
        '<div class="card" style="margin-bottom:20px"><div class="card-header"><h2>🔥 热度 Top 5</h2></div>' +
        '<ol class="top-list" id="topList"><li class="empty">加载中...</li></ol></div>' +
        '<div class="card"><div class="card-header"><h2>⚡ 操作</h2></div>' +
        '<div style="display:flex;gap:10px;align-items:center">' +
        '<button class="btn" onclick="doCompress()">🔄 批量压缩</button>' +
        '<span id="compressResult" style="color:var(--text-dim)"></span></div></div>';

    // Check Chart.js availability
    if (typeof Chart === 'undefined') {
        var chartsArea = document.getElementById('chartsArea');
        if (chartsArea) chartsArea.innerHTML = '<div class="card" style="grid-column:1/-1"><p style="color:var(--text-dim);text-align:center;padding:20px">图表加载失败（Chart.js不可用），请检查网络连接</p></div>';
    }

    api.getReport().then(function(report) {
        var total = report.total_count || 0;
        var bs = report.by_status || {};

        // Update stats using IDs
        var elTotal = document.getElementById('statTotal');
        var elActive = document.getElementById('statActive');
        var elDegraded = document.getElementById('statDegraded');
        var elArchived = document.getElementById('statArchived');
        if (elTotal) elTotal.textContent = total;
        if (elActive) elActive.textContent = bs.active || 0;
        if (elDegraded) elDegraded.textContent = bs.degraded || 0;
        if (elArchived) elArchived.textContent = bs.archived || 0;

        // Charts (only if Chart.js loaded)
        if (typeof Chart !== 'undefined') {
            var bc = report.by_category || {};
            renderCatChart(bc);
            renderStatusChart(bs);
        }

        // Top list
        var list = document.getElementById('topList');
        var top = (report.top_accessed || []).slice(0, 5);
        if (!list) return;
        if (top.length === 0) {
            list.innerHTML = '<li class="empty">暂无数据</li>';
            return;
        }
        list.innerHTML = top.map(function(m, i) {
            return '<li><span class="rank">' + (i + 1) + '</span>' +
                '<span class="info">' + catTag(m.category) + ' ' + escHtml(m.content.substring(0, 60)) + '</span>' +
                '<span class="count">' + m.access_count + 'x</span></li>';
        }).join('');
    }).catch(function(e) {
        showToast('加载报告失败: ' + e.message, 'error');
    });
}

function renderCatChart(data) {
    if (typeof Chart === 'undefined') return;
    if (dashCharts.cat) { try { dashCharts.cat.destroy(); } catch(e) {} dashCharts.cat = null; }
    var canvas = document.getElementById('catChart');
    if (!canvas) return;
    var labels = ['identity', 'principle', 'knowledge', 'working'];
    var values = labels.map(function(k) { return data[k] || 0; });
    var colors = ['#58a6ff', '#bc8cff', '#3fb950', '#d29922'];
    dashCharts.cat = new Chart(canvas, {
        type: 'doughnut',
        data: { labels: labels, datasets: [{ data: values, backgroundColor: colors, borderWidth: 0 }] },
        options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'bottom', labels: { color: '#8b949e', padding: 12 } } } }
    });
}

function renderStatusChart(data) {
    if (typeof Chart === 'undefined') return;
    if (dashCharts.status) { try { dashCharts.status.destroy(); } catch(e) {} dashCharts.status = null; }
    var canvas = document.getElementById('statusChart');
    if (!canvas) return;
    var labels = ['active', 'degraded', 'archived'];
    var values = labels.map(function(k) { return data[k] || 0; });
    var colors = ['#3fb950', '#d29922', '#f85149'];
    dashCharts.status = new Chart(canvas, {
        type: 'bar',
        data: { labels: labels, datasets: [{ data: values, backgroundColor: colors, borderWidth: 0, borderRadius: 4 }] },
        options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } }, scales: { x: { ticks: { color: '#8b949e' }, grid: { color: '#30363d' } }, y: { ticks: { color: '#8b949e' }, grid: { color: '#30363d' }, beginAtZero: true } } }
    });
}

function doCompress() {
    if (!confirm('执行批量压缩？将合并语义相似的working记忆。')) return;
    var el = document.getElementById('compressResult');
    if (el) el.textContent = '执行中...';
    api.compress().then(function(r) {
        showToast('压缩完成: ' + r.merged + ' merged, ' + r.archived + ' archived', 'success');
        if (el) el.textContent = '✅ ' + r.merged + ' merged, ' + r.archived + ' archived';
        renderDashboard();
    }).catch(function(e) {
        showToast('压缩失败: ' + e.message, 'error');
        if (el) el.textContent = '';
    });
}
