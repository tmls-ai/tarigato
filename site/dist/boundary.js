// The session-expiry illustration only, evaluated locally in the browser.
export function evaluateBoundary(expiry,repaired){
 if(!Number.isInteger(expiry)||expiry<98||expiry>102||typeof repaired!=='boolean') throw new TypeError('Expected an expiry from 98 to 102 and a repair flag.');
 const expected=expiry>100;
 const candidate=repaired?expected:expiry>=100;
 return {candidate,expected,matches:candidate===expected};
}
