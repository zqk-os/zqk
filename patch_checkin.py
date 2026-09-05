import re

content = open('pkg/agentclaim/checkin.go').read()
if '"math/rand/v2"' not in content and '"math/rand"' not in content:
    content = content.replace('"time"', '"math/rand"\n\t"time"')

rearm_func = """func Rearm(projectRoot string, timer *CheckinTimer, maxBackoff time.Duration) error {
	if timer == nil {
		return errfmt.Errorf("nil check-in timer")
	}
	backoff := timer.Cadence() * time.Duration(timer.Misses+1)
	if maxBackoff > 0 && backoff > maxBackoff {
		backoff = maxBackoff
	}
	
	// Add +/- 10% random jitter to avoid thundering herd on rearm
	jitter := time.Duration(float64(backoff) * 0.1 * (rand.Float64()*2 - 1))
	timer.ExpiresAt = time.Now().UTC().Add(backoff + jitter).Format(time.RFC3339)
	return writeCheckin(projectRoot, timer)
}"""

content = re.sub(r'func Rearm.*?\n}', rearm_func, content, flags=re.DOTALL)
open('pkg/agentclaim/checkin.go', 'w').write(content)
