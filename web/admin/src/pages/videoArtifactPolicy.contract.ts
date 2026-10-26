// @ts-nocheck
import { strict as assert } from 'node:assert';
import { videoArtifactHosts, videoArtifactHostsEnabled, normalizeVideoArtifactHost, videoArtifactHostSuggestions } from './videoArtifactPolicy.ts';
assert.equal(videoArtifactHostsEnabled(), false);
assert.equal(videoArtifactHostsEnabled({ video_artifact_hosts: ['cdn.example.com'] }), true);
assert.equal(videoArtifactHostsEnabled({ video_artifact_hosts_enabled: false, video_artifact_hosts: ['cdn.example.com'] }), false);
assert.equal(videoArtifactHostsEnabled({ video_artifact_hosts_enabled: true }), true);
assert.equal(videoArtifactHostsEnabled({ video_artifact_hosts_enabled: 'false' }), true);
assert.deepEqual(videoArtifactHosts({ video_artifact_hosts: [' CDN.example.com ', 'cdn.example.com', null, 42] }), ['cdn.example.com']);
assert.deepEqual(videoArtifactHosts(), []);
assert.equal(normalizeVideoArtifactHost(' Custom.Example.com '), 'custom.example.com');
for (const invalid of ['https://cdn.example.com', '*.example.com', 'cdn.example.com/path', 'user@cdn.example.com', 'cdn.example.com?x=1', 'cdn example.com']) assert.equal(normalizeVideoArtifactHost(invalid), '');
assert.equal(videoArtifactHostSuggestions.length, 2);
for (const host of videoArtifactHostSuggestions) assert.equal(normalizeVideoArtifactHost(host), host);
