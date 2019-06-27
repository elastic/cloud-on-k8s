#!/usr/bin/env python

import os
import json

project = os.environ['GCLOUD_PROJECT']

os.system('gcloud compute disks list --filter="-users:*" --format="json" --project {} > unused_disks.json'
          .format(project))

with open('unused_disks.json', 'r') as f:
    content = f.read()
    try:
        parsed_json_dict = json.loads(content)
        if len(parsed_json_dict) == 0:
            print("There is no unused disks. Congratulations!")
        else:
            for entry in parsed_json_dict:
                name = entry['name']
                head, tail = os.path.split(entry['zone'])
                os.system('gcloud compute disks delete {} --project {} --zone {} --quiet'
                      .format(name, project, tail))
    except:
        print("Can't parse JSON:")
        print(content)
