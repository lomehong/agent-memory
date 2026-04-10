/**
 * agents.js - Agent Management
 */
function renderAgents() {
    var content = document.getElementById('content');
    content.innerHTML = '<div class="card"><div class="card-header"><h2>🤖 Agent 管理</h2><button class="btn btn-primary" onclick="openAgentModal()">+ 注册 Agent</button></div>' +
        '<div class="table-wrap"><table><thead><tr><th>Agent ID</th><th>Name</th><th>Team</th><th>API Key</th><th>创建时间</th><th>操作</th></tr></thead>' +
        '<tbody id="agentTable"></tbody></table></div></div>';
    loadAgents();
}

function loadAgents() {
    api.listAgents().then(function(data) {
        var list = data.agents || [];
        var tbody = document.getElementById('agentTable');
        if (list.length === 0) { tbody.innerHTML = '<tr><td colspan="6" class="empty">暂无Agent</td></tr>'; return; }
        tbody.innerHTML = list.map(function(a) {
            var masked = (a.api_key_hash || '').substring(0, 12) + '...';
            return '<tr><td style="font-family:monospace;font-size:12px">' + escHtml(a.id) + '</td>' +
                '<td>' + escHtml(a.name) + '</td><td>' + escHtml(a.team || 'default') + '</td>' +
                '<td><span class="agent-key" data-full="' + escHtml(a.api_key_hash || '') + '">' + masked + '</span> ' +
                '<button class="btn btn-sm" onclick="toggleKey(this)" style="margin-left:4px;padding:1px 6px">👁</button></td>' +
                '<td>' + formatDate(a.created_at) + '</td>' +
                '<td><button class="btn btn-danger btn-sm" onclick="deleteAgent(\'' + a.id + '\')">删除</button></td></tr>';
        }).join('');
    }).catch(function(e) { showToast('加载Agent失败: ' + e.message, 'error'); });
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
