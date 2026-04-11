/**
 * app.js - Router, Toast, Modal, Utils
 */
var api = null;
var currentPage = 'dashboard';
var connected = false;
var token = null;
var currentUser = null;

function initApp() {
    token = localStorage.getItem('agentMemoryToken') || '';
    if (token) {
        api = new AgentMemoryAPI(window.location.origin, token);
        api.me().then(function(data) {
            connected = true;
            currentUser = data.username || data.user || '';
            document.getElementById('currentUser').textContent = currentUser;
            document.getElementById('logoutBtn').style.display = '';
            var status = document.getElementById('connStatus');
            status.className = 'conn-status connected';
            status.title = 'Connected';
            window.addEventListener('hashchange', handleRoute);
            handleRoute();
        }).catch(function() {
            token = '';
            localStorage.removeItem('agentMemoryToken');
            api = null;
            renderLoginPage();
            window.addEventListener('hashchange', handleRoute);
        });
    } else {
        renderLoginPage();
        window.addEventListener('hashchange', handleRoute);
    }
}

function renderLoginPage() {
    document.getElementById('currentUser').textContent = '';
    document.getElementById('logoutBtn').style.display = 'none';
    document.getElementById('connStatus').className = 'conn-status disconnected';
    document.getElementById('content').innerHTML =
        '<div class="login-container">' +
        '<div class="login-card">' +
        '<h2>🧠 Agent Memory</h2>' +
        '<div class="login-error" id="loginError"></div>' +
        '<div class="form-group"><label>用户名</label>' +
        '<input type="text" id="loginUsername" placeholder="请输入用户名" onkeydown="if(event.key===\'Enter\')document.getElementById(\'loginPassword\').focus()">' +
        '</div>' +
        '<div class="form-group"><label>密码</label>' +
        '<input type="password" id="loginPassword" placeholder="请输入密码" onkeydown="if(event.key===\'Enter\')doLogin()">' +
        '</div>' +
        '<button class="btn btn-primary" onclick="doLogin()">登 录</button>' +
        '<div class="login-footer">Agent Memory Management System</div>' +
        '</div></div>';
    setTimeout(function() {
        var el = document.getElementById('loginUsername');
        if (el) el.focus();
    }, 100);
}

function doLogin() {
    var username = document.getElementById('loginUsername').value.trim();
    var password = document.getElementById('loginPassword').value;
    if (!username || !password) {
        document.getElementById('loginError').textContent = '请输入用户名和密码';
        return;
    }
    document.getElementById('loginError').textContent = '';
    var tempApi = new AgentMemoryAPI(window.location.origin, '');
    tempApi.login(username, password).then(function(data) {
        token = data.token;
        localStorage.setItem('agentMemoryToken', token);
        api = new AgentMemoryAPI(window.location.origin, token);
        currentUser = data.username || username;
        document.getElementById('currentUser').textContent = currentUser;
        document.getElementById('logoutBtn').style.display = '';
        connected = true;
        var status = document.getElementById('connStatus');
        status.className = 'conn-status connected';
        status.title = 'Connected';
        window.location.hash = '#dashboard';
        handleRoute();
    }).catch(function(err) {
        document.getElementById('loginError').textContent = err.message || '登录失败';
    });
}

function doLogout() {
    if (api) {
        api.logout().catch(function() {});
    }
    token = '';
    currentUser = '';
    connected = false;
    api = null;
    localStorage.removeItem('agentMemoryToken');
    document.getElementById('currentUser').textContent = '';
    document.getElementById('logoutBtn').style.display = 'none';
    document.getElementById('connStatus').className = 'conn-status disconnected';
    renderLoginPage();
}

function checkConnection() {
    var status = document.getElementById('connStatus');
    if (!api || !token) {
        status.className = 'conn-status disconnected';
        return;
    }
    api.me().then(function(data) {
        connected = true;
        currentUser = data.username || data.user || currentUser;
        document.getElementById('currentUser').textContent = currentUser;
        status.className = 'conn-status connected';
        status.title = 'Connected';
        handleRoute();
    }).catch(function() {
        connected = false;
        token = '';
        localStorage.removeItem('agentMemoryToken');
        api = null;
        status.className = 'conn-status disconnected';
        status.title = 'Disconnected';
        renderLoginPage();
        showToast('登录已过期，请重新登录', 'error');
    });
}

function handleRoute() {
    if (!connected) {
        renderLoginPage();
        return;
    }
    var hash = window.location.hash.slice(1) || 'dashboard';
    currentPage = hash;

    var navItems = document.querySelectorAll('.nav-item');
    for (var i = 0; i < navItems.length; i++) {
        var el = navItems[i];
        if (el.getAttribute('data-page') === hash) {
            el.classList.add('active');
        } else {
            el.classList.remove('active');
        }
    }

    switch (hash) {
        case 'memories': renderMemories(); break;
        case 'agents': renderAgents(); break;
        case 'system': renderSystem(); break;
        default: renderDashboard(); break;
    }
}

function toggleSidebar() {
    document.getElementById('sidebar').classList.toggle('open');
}

function showToast(msg, type) {
    type = type || 'info';
    var el = document.createElement('div');
    el.className = 'toast ' + type;
    el.textContent = msg;
    document.getElementById('toastContainer').appendChild(el);
    setTimeout(function() { el.remove(); }, 3000);
}

function openModal(title, bodyHtml, width) {
    document.getElementById('modalTitle').textContent = title;
    document.getElementById('modalBody').innerHTML = bodyHtml;
    var modal = document.querySelector('.modal');
    modal.style.width = width || '560px';
    document.getElementById('modalOverlay').classList.add('active');
}

function closeModal() {
    document.getElementById('modalOverlay').classList.remove('active');
}

function escHtml(s) {
    if (!s) return '';
    var d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
}

function formatDate(str) {
    if (!str) return '-';
    try { return new Date(str).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }); }
    catch (e) { return str; }
}

function formatDuration(seconds) {
    var h = Math.floor(seconds / 3600);
    var m = Math.floor((seconds % 3600) / 60);
    if (h > 0) return h + 'h ' + m + 'm';
    return m + 'm';
}

function formatBytes(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / 1048576).toFixed(1) + ' MB';
}

function catTag(cat) { return '<span class="tag tag-' + cat + '">' + cat + '</span>'; }
function statusTag(s) { return '<span class="tag tag-' + s + '">' + s + '</span>'; }
function visTag(v) { return '<span class="tag tag-' + v + '">' + v + '</span>'; }

function scoreBar(score) {
    var pct = Math.round((score || 0) * 100);
    return '<div class="score-bar"><div class="score-bar-fill" style="width:' + pct + '%"></div><span class="score-text">' + pct + '%</span></div>';
}

document.addEventListener('DOMContentLoaded', initApp);
