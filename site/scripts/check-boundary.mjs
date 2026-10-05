import assert from 'node:assert/strict';
import {evaluateBoundary} from '../dist/boundary.js';
for(const [expiry,candidate,expected,matches] of [[98,false,false,true],[99,false,false,true],[100,true,false,false],[101,true,true,true],[102,true,true,true]]){
 assert.deepEqual(evaluateBoundary(expiry,false),{candidate,expected,matches});
 assert.deepEqual(evaluateBoundary(expiry,true),{candidate:expected,expected,matches:true});
}
for(const value of [97,103,99.5,NaN,'100']) assert.throws(()=>evaluateBoundary(value,false),TypeError);
assert.throws(()=>evaluateBoundary(100,'true'),TypeError);
console.log('Boundary values, exact expiry, repair, and invalid inputs passed.');
