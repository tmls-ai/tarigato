import {evaluateBoundary} from './boundary.js';
// Display data for the fixed browser simulation; no agents or tests execute.
export function inspectSource(scenario,through,expiry){
 const lines=sourceLines(scenario,through);
 return {lines,...evaluateBoundary(expiry,!lines[3].includes('>='))};
}
export function sourceLines(scenario,through){
 const fixed=through.includes('builder')&&(scenario!=='repair'||through.includes('repair'));
 return ['package session','','func Valid(expiresAt, now int64) bool {',`    return expiresAt ${fixed?'>':'>='} now`,'}'];
}
export function challengeLines(scenario){
 if(scenario==='inconclusive')return ['package session','','import ("testing"; "time")','','// Illustrative unstable test','func TestExpiryTiming(t *testing.T) {','    expiresAt := time.Now().Unix() + 1','    time.Sleep(900 * time.Millisecond)','    if Valid(expiresAt, time.Now().Unix()) {','        t.Fatal("expired session accepted")','    }','}'];
 return ['package session','','import "testing"','','func TestRejectAtExpiry(t *testing.T) {','    if Valid(100, 100) {','        t.Fatal("expired session accepted")','    }','}'];
}
