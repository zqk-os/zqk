import glob, yaml

for f in glob.glob('docs/process/agent_tasks/*.yaml'):
    with open(f, 'r') as file:
        data = yaml.safe_load(file)
    
    status = data.get('status', '')
    if status in ['approved', 'in_progress', 'pending_verification']:
        branch = data.get('branch_ref', '')
        # If it's missing or set to main, fix it
        if branch == 'main' or not branch:
            data['branch_ref'] = 'integration/pri-cef-r28-remediate-001'
            with open(f, 'w') as file:
                yaml.dump(data, file, sort_keys=False)
            print(f"Updated {data['id']}")
