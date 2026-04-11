/**
 * memories.js - Memory Management
 */
var memState = { offset: 0, limit: 20, total: 0, filter: {}, selected: new Set() };

function renderMemories() {
    var content = document.getElementById('content');
    content.innerHTML = '<div class="card"><div class="card-header"><h2>📝 记忆管理</h2><button class="btn btn-primary" onclick="openCreateModal()">+ 新建记忆</button></div>' +
        '<div class="filters"><div class="search-box"><input type="text" id="searchInput" placeholder="语义搜索..." onkeydown="if(event.key===\'Enter\')doSearch()">' +
        '<button class="btn" onclick="doSearch()">搜索</button></div>' +
        '<select id="filterCat" onchange="applyFilter()"><option value="">全部Category</option><option value="identity">identity</option><option value="principle">principle</option><option value="knowledge">knowledge</option><option value="working">working</option></select>' +
        '<select id="filterStatus" onchange="applyFilter()"><option value="">全部Status</option><option value="active">active</option><option value="degraded">degraded</option><option value="archived">archived</option></select></div>' +
        '<div class="table-wrap"><table><thead><tr><th><input type="checkbox" onchange="toggleAll(this)"></th><th>内容</th><th>Agent</th><th>分类</th><th>优先级</th><th>状态</th><th>可见性</th><th>访问</th><th>创建时间</th><th>操作</th></tr></thead>' +
        '<tbody id="memTable"></tbody></table></div>' +
        '<div class="pagination"><span id="memInfo"></span><div><button class="btn btn-sm" onclick="memPrev()">« 上一页</button><button class="btn btn-sm" onclick="memNext()">下一页 »</button></div></div>' +
        '<div style="margin-top:8px"><button class="btn btn-danger btn-sm" id="batchDelBtn" style="display:none" onclick="batchDelete()">批量删除 (<span id="batchCount">0</span>)</button></div></div>';
    memState.offset = 0;
    memState.selected.clear();
    loadMemories();
}

function loadMemories() {
    var opts = { limit: memState.limit, offset: memState.offset };
    var cat = document.getElementById('filterCat').value;
    var status = document.getElementById('filterStatus').value;
    if (cat) opts.category = cat;
    if (status) opts.status = status;
    memState.filter = opts;

    api.listMemories(opts).then(function(data) {
        memState.total = data.total || data.count || 0;
        var tbody = document.getElementById('memTable');
        var list = data.memories || [];
        if (list.length === 0) { tbody.innerHTML = '<tr><td colspan="10" class="empty">暂无记忆</td></tr>'; }
        else {
            tbody.innerHTML = list.map(function(m) {
                var checked = memState.selected.has(m.id) ? 'checked' : '';
                return '<tr><td><input type="checkbox" data-id="' + m.id + '" ' + checked + ' onchange="toggleSelect(\'' + m.id + '\',this.checked)"></td>' +
                    '<td class="content-cell">' + escHtml(m.content) + '</td>' +
                    '<td><span class="tag tag-agent" title="user_id: ' + escHtml(m.user_id || '') + '">' + escHtml(m.agent_id || '-') + '</span></td>' +
                    '<td>' + catTag(m.category) + '</td><td>' + m.priority + '</td><td>' + statusTag(m.status) + '</td>' +
                    '<td>' + visTag(m.visibility) + '</td><td>' + m.access_count + '</td>' +
                    '<td>' + formatDate(m.created_at) + '</td>' +
                    '<td><button class="btn btn-sm" onclick="openEditModal(\'' + m.id + '\')">编辑</button></td></tr>';
            }).join('');
        }
        updatePagination();
    }).catch(function(e) { showToast('加载失败: ' + e.message, 'error'); });
}

function updatePagination() {
    var pages = Math.ceil(memState.total / memState.limit);
    var cur = Math.floor(memState.offset / memState.limit) + 1;
    document.getElementById('memInfo').textContent = '共 ' + memState.total + ' 条，第 ' + cur + '/' + Math.max(pages,1) + ' 页';
}

function memPrev() { if (memState.offset > 0) { memState.offset -= memState.limit; loadMemories(); } }
function memNext() { if (memState.offset + memState.limit < memState.total) { memState.offset += memState.limit; loadMemories(); } }
function applyFilter() { memState.offset = 0; memState.selected.clear(); loadMemories(); }

function toggleAll(el) {
    var cbs = document.querySelectorAll('#memTable input[type=checkbox]');
    cbs.forEach(function(cb) { if (cb.dataset.id) { cb.checked = el.checked; toggleSelect(cb.dataset.id, el.checked); } });
}

function toggleSelect(id, checked) {
    if (checked) memState.selected.add(id); else memState.selected.delete(id);
    var btn = document.getElementById('batchDelBtn');
    document.getElementById('batchCount').textContent = memState.selected.size;
    btn.style.display = memState.selected.size > 0 ? 'inline-flex' : 'none';
}

function batchDelete() {
    if (!confirm('确认删除 ' + memState.selected.size + ' 条记忆？')) return;
    var ids = Array.from(memState.selected);
    var p = Promise.all(ids.map(function(id) { return api.deleteMemory(id); }));
    p.then(function() { showToast('已删除 ' + ids.length + ' 条记忆', 'success'); memState.selected.clear(); loadMemories(); })
     .catch(function(e) { showToast('删除失败: ' + e.message, 'error'); });
}

function doSearch() {
    var q = document.getElementById('searchInput').value.trim();
    if (!q) { showToast('请输入搜索内容', 'info'); return; }
    var cat = document.getElementById('filterCat').value;
    api.searchMemories(q, cat || undefined, 20).then(function(data) {
        memState.total = data.count || 0;
        var results = data.results || [];
        var tbody = document.getElementById('memTable');
        if (results.length === 0) { tbody.innerHTML = '<tr><td colspan="10" class="empty">无匹配结果</td></tr>'; }
        else {
            tbody.innerHTML = results.map(function(m) {
                return '<tr><td></td><td class="content-cell">' + escHtml(m.content) + '</td>' +
                    '<td><span class="tag tag-agent" title="user_id: ' + escHtml(m.user_id || '') + '">' + escHtml(m.agent_id || '-') + '</span></td>' +
                    '<td>' + catTag(m.category) + '</td>' +
                    '<td>' + m.priority + '</td><td>' + statusTag(m.status) + '</td><td>' + visTag(m.visibility) + '</td>' +
                    '<td>' + scoreBar(m.score) + '</td><td>' + formatDate(m.created_at) + '</td>' +
                    '<td><button class="btn btn-sm" onclick="openEditModal(\'' + m.id + '\')">编辑</button></td></tr>';
            }).join('');
        }
        document.getElementById('memInfo').textContent = '搜索结果: ' + memState.total + ' 条';
    }).catch(function(e) { showToast('搜索失败: ' + e.message, 'error'); });
}

function openCreateModal() {
    var html = '<div class="form-group"><label>内容 *</label><textarea id="mContent" rows="3" placeholder="输入记忆内容..."></textarea></div>' +
        '<div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">' +
        '<div class="form-group"><label>分类</label><select id="mCat"><option value="">自动推断</option><option value="identity">identity</option><option value="principle">principle</option><option value="knowledge">knowledge</option><option value="working">working</option></select></div>' +
        '<div class="form-group"><label>优先级</label><select id="mPri"><option value="">自动</option><option value="1">1 (最高)</option><option value="2">2</option><option value="3">3</option><option value="4">4</option><option value="5">5</option></select></div>' +
        '<div class="form-group"><label>可见性</label><select id="mVis"><option value="">自动推断</option><option value="private">private</option><option value="team">team</option><option value="user">user</option></select></div>' +
        '<div class="form-group"><label>TTL</label><select id="mTtl"><option value="">自动</option><option value="permanent">permanent</option><option value="year">year</option><option value="month">month</option><option value="week">week</option></select></div></div>' +
        '<div class="form-group"><label>标签 (逗号分隔)</label><input type="text" id="mTags" placeholder="tag1, tag2"></div>' +
        '<div id="createSuggestion"></div>' +
        '<div class="modal-footer"><button class="btn btn-primary" onclick="doCreate()">创建</button><button class="btn" onclick="closeModal()">取消</button></div>';
    openModal('新建记忆', html);
}

function doCreate() {
    var content = document.getElementById('mContent').value.trim();
    if (!content) { showToast('请输入内容', 'error'); return; }
    var data = { content: content };
    var cat = document.getElementById('mCat').value; if (cat) data.category = cat;
    var pri = document.getElementById('mPri').value; if (pri) data.priority = parseInt(pri);
    var vis = document.getElementById('mVis').value; if (vis) data.visibility = vis;
    var ttl = document.getElementById('mTtl').value; if (ttl) data.ttl = ttl;
    var tags = document.getElementById('mTags').value.trim();
    if (tags) data.tags = tags.split(',').map(function(t) { return t.trim(); });

    api.createMemory(data).then(function(result) {
        var sug = result.suggestion || {};
        var msg = sug.dedup_hit ? '已合并到 ' + sug.dedup_memory_id + ' (score: ' + Math.round(sug.dedup_score*100) + '%)' : '创建成功';
        showToast(msg, 'success');
        closeModal();
        loadMemories();
    }).catch(function(e) { showToast('创建失败: ' + e.message, 'error'); });
}

function openEditModal(id) {
    api.getMemory(id).then(function(m) {
        var html = '<div class="form-group"><label>内容</label><textarea id="eContent" rows="3">' + escHtml(m.content) + '</textarea></div>' +
            '<div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">' +
            '<div class="form-group"><label>分类</label><select id="eCat">' +
            ['identity','principle','knowledge','working'].map(function(c) { return '<option value="'+c+'"'+(m.category===c?' selected':'')+'>'+c+'</option>'; }).join('') +
            '</select></div><div class="form-group"><label>优先级</label><input type="number" id="ePri" min="1" max="5" value="' + m.priority + '"></div>' +
            '<div class="form-group"><label>可见性</label><select id="eVis">' +
            ['private','team','user'].map(function(v) { return '<option value="'+v+'"'+(m.visibility===v?' selected':'')+'>'+v+'</option>'; }).join('') +
            '</select></div><div class="form-group"><label>TTL</label><select id="eTtl">' +
            ['permanent','year','month','week','session'].map(function(t) { return '<option value="'+t+'"'+(m.ttl===t?' selected':'')+'>'+t+'</option>'; }).join('') +
            '</select></div></div>' +
            '<div class="form-group"><label>标签</label><input type="text" id="eTags" value="' + (m.tags || []).join(', ') + '"></div>' +
            '<div style="color:var(--text-dim);font-size:12px;margin-bottom:12px;padding:8px;background:var(--bg);border-radius:4px">' +
            '🤖 Agent: <strong>' + escHtml(m.agent_id || '-') + '</strong> &nbsp;|&nbsp; 👤 User: ' + escHtml(m.user_id || '-') + ' &nbsp;|&nbsp; 👥 Team: ' + escHtml(m.team || '-') +
            '<br>ID: ' + m.id + ' | 版本: ' + m.version + ' | 创建: ' + formatDate(m.created_at) + ' | 访问: ' + m.access_count + '次</div>' +
            '<div class="modal-footer"><button class="btn btn-primary" onclick="doUpdate(\'' + m.id + '\')">保存</button>' +
            '<button class="btn btn-danger" onclick="doDelete(\'' + m.id + '\')">删除</button><button class="btn" onclick="closeModal()">关闭</button></div>';
        openModal('编辑记忆', html);
    }).catch(function(e) { showToast('加载失败: ' + e.message, 'error'); });
}

function doUpdate(id) {
    var data = {
        content: document.getElementById('eContent').value,
        category: document.getElementById('eCat').value,
        priority: parseInt(document.getElementById('ePri').value),
        visibility: document.getElementById('eVis').value,
        ttl: document.getElementById('eTtl').value,
        tags: document.getElementById('eTags').value.split(',').map(function(t) { return t.trim(); }).filter(Boolean)
    };
    api.updateMemory(id, data).then(function() { showToast('已保存', 'success'); closeModal(); loadMemories(); })
     .catch(function(e) { showToast('保存失败: ' + e.message, 'error'); });
}

function doDelete(id) {
    if (!confirm('确认删除此记忆？此操作不可撤销。')) return;
    api.deleteMemory(id).then(function() { showToast('已删除', 'success'); closeModal(); loadMemories(); })
     .catch(function(e) { showToast('删除失败: ' + e.message, 'error'); });
}
