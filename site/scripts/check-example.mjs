import assert from 'node:assert/strict';
import {sourceLines,challengeLines,inspectSource} from '../dist/example.js';
for(const [scenario,moves,statement] of [
 ['pass',[],'    return expiresAt >= now'],
 ['pass',['builder'],'    return expiresAt > now'],
 ['repair',['builder'],'    return expiresAt >= now'],
 ['repair',['builder','repair'],'    return expiresAt > now'],
 ['inconclusive',['builder'],'    return expiresAt > now']
]) assert.equal(sourceLines(scenario,moves)[3],statement);
assert.match(challengeLines('pass').join('\n'),/func TestRejectAtExpiry.*\n    if Valid\(100, 100\)/);
assert.deepEqual(challengeLines('pass'),challengeLines('repair'));
assert.match(challengeLines('inconclusive').join('\n'),/time.Sleep/);
for(const scenario of ['pass','repair','inconclusive']){
 for(const expiry of [99,100,101]){
  const starting=inspectSource(scenario,[],expiry);
  assert.equal(starting.matches,expiry!==100);
  const candidate=inspectSource(scenario,['builder'],expiry);
  assert.equal(candidate.matches,scenario!=='repair'||expiry!==100);
  assert.equal(inspectSource(scenario,['builder','repair'],expiry).matches,true);
 }
}
console.log('Candidate, frozen challenge, repair, and unstable-test display cases passed.');
