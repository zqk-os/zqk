import json, hashlib, os, glob

index_file = 'docs/process/agent_tasks/.agent_task.index'
with open(index_file, 'r') as f:
    idx = json.load(f)

changed = False
for fpath in glob.glob('docs/process/agent_tasks/*.yaml'):
    with open(fpath, 'rb') as f:
        content = f.read()
    
    actual_hash = hashlib.sha256(content).hexdigest()
    fname = os.path.basename(fpath)
    expected_hash = fname.split('.')[0]
    
    if actual_hash != expected_hash:
        print(f"Fixing {fname} -> {actual_hash}.yaml")
        new_path = f"docs/process/agent_tasks/{actual_hash}.yaml"
        os.rename(fpath, new_path)
        
        # update index mapping
        for k, v in idx.items():
            if v == expected_hash:
                idx[k] = actual_hash
                changed = True

if changed:
    with open(index_file, 'w') as f:
        json.dump(idx, f, indent=2)
    print("Updated cas index.")
