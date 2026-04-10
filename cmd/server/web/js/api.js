/**
 * api.js - API Client
 */
function AgentMemoryAPI(baseUrl, token) {
    this.baseUrl = baseUrl;
    this.token = token;
}

AgentMemoryAPI.prototype.request = function(method, path, body) {
    var opts = {
        method: method,
        headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + this.token }
    };
    if (body !== undefined) opts.body = JSON.stringify(body);
    return fetch(this.baseUrl + path, opts).then(function(resp) {
        if (!resp.ok) return resp.json().then(function(d) { throw new Error(d.error || 'HTTP ' + resp.status); });
        return resp.json();
    });
};

AgentMemoryAPI.prototype.health = function() { return this.request('GET', '/api/v1/health'); };
AgentMemoryAPI.prototype.systemConfig = function() { return this.request('GET', '/api/v1/system/config'); };

AgentMemoryAPI.prototype.searchMemories = function(query, category, topK) {
    var p = '?query=' + encodeURIComponent(query);
    if (category) p += '&category=' + category;
    if (topK) p += '&top_k=' + topK;
    return this.request('GET', '/api/v1/memories/search' + p);
};

AgentMemoryAPI.prototype.listMemories = function(opts) {
    opts = opts || {};
    var p = '?limit=' + (opts.limit || 20) + '&offset=' + (opts.offset || 0);
    if (opts.category) p += '&category=' + opts.category;
    if (opts.status) p += '&status=' + opts.status;
    if (opts.visibility) p += '&visibility=' + opts.visibility;
    return this.request('GET', '/api/v1/memories' + p);
};

AgentMemoryAPI.prototype.getMemory = function(id) { return this.request('GET', '/api/v1/memories/' + id); };
AgentMemoryAPI.prototype.createMemory = function(data) { return this.request('POST', '/api/v1/memories', data); };
AgentMemoryAPI.prototype.updateMemory = function(id, data) { return this.request('PUT', '/api/v1/memories/' + id, data); };
AgentMemoryAPI.prototype.deleteMemory = function(id) { return this.request('DELETE', '/api/v1/memories/' + id); };
AgentMemoryAPI.prototype.batchCreate = function(items) { return this.request('POST', '/api/v1/memories/batch', items); };
AgentMemoryAPI.prototype.compress = function() { return this.request('POST', '/api/v1/memories/compress', {}); };
AgentMemoryAPI.prototype.getReport = function() { return this.request('GET', '/api/v1/memories/report'); };

AgentMemoryAPI.prototype.listAgents = function() { return this.request('GET', '/api/v1/agents'); };
AgentMemoryAPI.prototype.createAgent = function(data) { return this.request('POST', '/api/v1/agents', data); };
AgentMemoryAPI.prototype.deleteAgent = function(id) { return this.request('DELETE', '/api/v1/agents/' + id); };

AgentMemoryAPI.prototype.login = function(username, password) {
    return fetch(this.baseUrl + '/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: username, password: password })
    }).then(function(resp) {
        if (!resp.ok) return resp.json().then(function(d) { throw new Error(d.error || '登录失败'); });
        return resp.json();
    });
};

AgentMemoryAPI.prototype.logout = function() {
    return this.request('POST', '/api/v1/auth/logout', {});
};

AgentMemoryAPI.prototype.me = function() {
    return this.request('GET', '/api/v1/auth/me');
};
