/**
 * agents.js - Agent Management
 */
function renderAgents() {
    var content = document.getElementById('content');
    content.innerHTML = '<div class="card"><div class="card-header"><h2>🤖 Agent 管理</h2><button class="btn btn-primary" onclick="openAgentModal()">+ 注册 Agent</button></div>' +
        '<div class="table-wrap"><table><thead><tr><th>Agent ID</th><th>Name</th><th>Team</th><th>User ID</th><th>记忆数</th><th>API Key</th><th>创建时间</th><th>操作</th></tr></thead>' +
        '<tbody id="agentTable"></tbody></table></div></div>';
    loadAgents();
}

function loadAgents() {
    Promise.all([api.listAgents(), api.getReport()]).then(function(results) {
        var agents = results[0].agents || [];
        var report = results[1] || {};

        // Build agent_id -> memory count map from report
        var memCounts = {};
        var memByAgent = {};
        (report.top_accessed || []).forEach(function(m) {
            var aid = m.agent_id || 'unknown';
            memCounts[aid] = (memCounts[aid] || 0) + 1;
            if (!memByAgent[aid]) memByAgent[aid] = [];
        });

        var tbody = document.getElementById('agentTable');
        if (agents.length === 0) { tbody.innerHTML = '<tr><td colspan="8" class="empty">暂无Agent</td></tr>'; return; }
        tbody.innerHTML = agents.map(function(a) {
            var masked = (a.api_key_hash || '').substring(0, 12) + '...';
            var count = memCounts[a.id] || 0;
            return '<tr>' +
                '<td style="font-family:monospace;font-size:12px">' + escHtml(a.id) + '</td>' +
                '<td>' + escHtml(a.name) + '</td>' +
                '<td>' + escHtml(a.team || 'default') + '</td>' +
                '<td style="font-family:monospace;font-size:12px">' + escHtml(a.user_id || '-') + '</td>' +
                '<td><span class="tag tag-agent">' + count + '</span></td>' +
                '<td><span class="agent-key" data-full="' + escHtml(a.api_key_hash || '') + '">' + masked + '</span> ' +
                '<button class="btn btn-sm" onclick="toggleKey(this)" style="margin-left:4px;padding:1px 6px">👁</button></td>' +
                '<td>' + formatDate(a.created_at) + '</td>' +
                '<td>' +
                '<button class="btn btn-sm" onclick="viewAgentMemories(\'' + escHtml(a.id) + '\',\'' + escHtml(a.name || a.id) + '\',\'' + escHtml(a.user_id || '') + '\')">📝 记忆</button> ' +
                '<button class="btn btn-danger btn-sm" onclick="deleteAgent(\'' + a.id + '\')">删除</button></td></tr>';
        }).join('');
    }).catch(function(e) { showToast('加载Agent失败: ' + e.message, 'error'); });
}

function viewAgentMemories(agentId, agentName, userId) {
    var html = '<div style="margin-bottom:12px;color:var(--text-dim);font-size:13px">🤖 <strong>' + escHtml(agentName) + '</strong> (' + escHtml(agentId) + ') 的所有记忆</div>' +
        '<div id="agentMemList" style="max-height:500px;overflow-y:auto"><div class="empty">加载中...</div></div>' +
        '<div class="modal-footer"><button class="btn" onclick="closeModal()">关闭</button></div>';
    openModal('📝 ' + agentName + ' - 记忆列表', html, '860px');

    // Load all memories for this agent
    api.listMemories({ limit: 200 }).then(function(data) {
        var all = (data.memories || []).filter(function(m) { return m.agent_id === agentId; });
        var container = document.getElementById('agentMemList');
        if (!container) return;
        if (all.length === 0) {
            container.innerHTML = '<div class="empty">该 Agent 暂无记忆</div>';
            return;
        }
        container.innerHTML = '<table style="width:100%;font-size:13px;border-collapse:collapse">' +
            '<thead><tr style="background:var(--bg)"><th style="text-align:left;padding:6px 8px">内容</th><th style="width:60px">分类</th><th style="width:40px">优先级</th><th style="width:50px">可见性</th><th style="width:40px">访问</th><th style="width:50px">详情</th></tr></thead>' +
            '<tbody>' + all.map(function(m) {
                return '<tr style="border-bottom:1px solid var(--border)">' +
                    '<td style="padding:6px 8px;max-width:480px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="' + escHtml(m.content) + '">' + escHtml(m.content) + '</td>' +
                    '<td style="padding:6px 4px;text-align:center">' + catTag(m.category) + '</td>' +
                    '<td style="padding:6px 4px;text-align:center">' + m.priority + '</td>' +
                    '<td style="padding:6px 4px;text-align:center">' + visTag(m.visibility) + '</td>' +
                    '<td style="padding:6px 4px;text-align:center">' + m.access_count + '</td>' +
                    '<td style="padding:6px 4px;text-align:center"><button class="btn btn-sm" onclick="viewMemoryDetail(\'' + m.id + '\')">查看</button></td></tr>';
            }).join('') + '</tbody></table>' +
            '<div style="margin-top:8px;color:var(--text-dim);font-size:12px">共 ' + all.length + ' 条记忆</div>';
    }).catch(function(e) {
        var container = document.getElementById('agentMemList');
        if (container) container.innerHTML = '<div class="empty" style="color:var(--error)">加载失败: ' + escHtml(e.message) + '</div>';
    });
}

function viewMemoryDetail(id) {
    api.getMemory(id).then(function(m) {
        var tags = (m.tags || []).join(', ');
        var html =
            '<div style="margin-bottom:12px">' +
            '<div class="form-group"><label>内容</label><div style="padding:10px;background:var(--bg);border-radius:4px;white-space:pre-wrap;line-height:1.6">' + escHtml(m.content) + '</div></div>' +
            '<div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">' +
            '<div class="form-group"><label>分类</label><div>' + catTag(m.category) + '</div></div>' +
            '<div class="form-group"><label>优先级</label><div>' + m.priority + '</div></div>' +
            '<div class="form-group"><label>状态</label><div>' + statusTag(m.status) + '</div></div>' +
            '<div class="form-group"><label>可见性</label><div>' + visTag(m.visibility) + '</div></div>' +
            '<div class="form-group"><label>TTL</label><div>' + escHtml(m.ttl || '-') + '</div></div>' +
            '<div class="form-group"><label>置信度</label><div>' + (m.confidence || '-') + '</div></div>' +
            '<div class="form-group"><label>标签</label><div>' + escHtml(tags || '-') + '</div></div>' +
            '<div class="form-group"><label>访问次数</label><div>' + m.access_count + '</div></div>' +
            '</div>' +
            '<div style="margin-top:12px;padding:10px;background:var(--bg);border-radius:4px;font-size:12px;color:var(--text-dim)">' +
            '🤖 Agent: <strong>' + escHtml(m.agent_id || '-') + '</strong> &nbsp;|&nbsp; 👤 User: ' + escHtml(m.user_id || '-') + ' &nbsp;|&nbsp; 👥 Team: ' + escHtml(m.team || '-') +
            '<br>📌 ID: ' + m.id + ' &nbsp;|&nbsp; 版本: ' + m.version +
            '<br>创建: ' + formatDate(m.created_at) + ' &nbsp;|&nbsp; 更新: ' + formatDate(m.updated_at) + ' &nbsp;|&nbsp; 最近访问: ' + formatDate(m.last_accessed) +
            '</div>' +
            '<div class="modal-footer" style="margin-top:16px"><button class="btn btn-primary" onclick="closeModal();openEditModal(\'' + m.id + '\')">编辑</button><button class="btn" onclick="closeModal()">关闭</button></div>';
        openModal('📝 记忆详情', html, '600px');
    }).catch(function(e) { showToast('加载失败: ' + e.message, 'error'); });
}

function toggleKey(btn) {
    var span = btn.previousElementSibling;
    if (span.dataset.shown === '1') {
        var masked = (span.dataset.full).substring(0, 12) + '...';
        span.textContent = masked;
        span.dataset.shown = '0';
    } else {
        span.textContent = span.dataset.full;
        span.dataset.shown = '1';
    }
}

function openAgentModal() {
    var newKey = 'ak-' + Math.random().toString(36).substring(2, 14);
    var html = '<div class="form-group"><label>Name *</label><input type="text" id="aName" placeholder="Agent名称"></div>' +
        '<div class="form-group"><label>Team</label><input type="text" id="aTeam" value="default"></div>' +
        '<div class="form-group"><label>API Key</label><div style="display:flex;gap:8px"><input type="text" id="aKey" value="' + newKey + '">' +
        '<button class="btn btn-sm" onclick="document.getElementById(\'aKey\').value=\'ak-\'+Math.random().toString(36).substring(2,14)">🔄</button></div></div>' +
        '<p style="color:var(--degraded);font-size:12px;margin-top:8px">⚠️ API Key 创建后请妥善保存，此密钥用于Agent认证</p>' +
        '<div class="modal-footer"><button class="btn btn-primary" onclick="doCreateAgent()">注册</button><button class="btn" onclick="closeModal()">取消</button></div>';
    openModal('注册 Agent', html);
}

function doCreateAgent() {
    var name = document.getElementById('aName').value.trim();
    if (!name) { showToast('请输入Agent名称', 'error'); return; }
    var team = document.getElementById('aTeam').value.trim() || 'default';
    api.createAgent({ name: name, team: team }).then(function(a) {
        showToast('Agent ' + a.name + ' 已注册', 'success');
        closeModal();
        loadAgents();
    }).catch(function(e) { showToast('注册失败: ' + e.message, 'error'); });
}

function deleteAgent(id) {
    if (!confirm('确认删除此Agent？')) return;
    api.deleteAgent(id).then(function() { showToast('已删除', 'success'); loadAgents(); })
     .catch(function(e) { showToast('删除失败: ' + e.message, 'error'); });
}
