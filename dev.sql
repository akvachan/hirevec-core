-- users
insert into users (id, provider, provider_user_id, email, full_name, user_name)
values
  ('usr_01hzzzcandidate000000000000', 'google', 'cand_001', 'candidate@example.com', 'Test Candidate', 'test_candidate'),
  ('usr_01hzzzrecruiter00000000000', 'google', 'rec_001', 'recruiter@example.com', 'Test Recruiter', 'test_recruiter')
on conflict (id) do nothing;

-- candidate
insert into candidates (id, user_id, about)
values
  ('can_01hzzzcand00000000000000000', 'usr_01hzzzcandidate000000000000', 'Backend engineer focused on distributed systems, APIs, and database design.')
on conflict (id) do nothing;

-- recruiter
insert into recruiters (id, user_id)
values
  ('rec_01hzzzrecruiterrole0000000', 'usr_01hzzzrecruiter00000000000')
on conflict (id) do nothing;

-- positions (a couple examples)
insert into positions (id, recruiter_id, title, description, company)
values
  ('pos_01hzzzpos00000000000000004', 'rec_01hzzzrecruiterrole0000000', 'Backend Engineer 1', 'Scaling distributed systems and APIs.', 'Initech'),
  ('pos_01hzzzpos00000000000000005', 'rec_01hzzzrecruiterrole0000000', 'Backend Engineer 2', 'Designing microservices and data pipelines.', 'Acme Corp'),
  ('pos_01hzzzpos00000000000000006', 'rec_01hzzzrecruiterrole0000000', 'Backend Engineer 3', 'Event-driven architecture and Kafka systems.', 'Globex Inc'),
  ('pos_01hzzzpos00000000000000007', 'rec_01hzzzrecruiterrole0000000', 'Backend Engineer 4', 'High-performance APIs and caching strategies.', 'Initech'),
  ('pos_01hzzzpos00000000000000008', 'rec_01hzzzrecruiterrole0000000', 'Backend Engineer 5', 'Database design and query optimization.', 'Acme Corp'),
  ('pos_01hzzzpos00000000000000009', 'rec_01hzzzrecruiterrole0000000', 'Backend Engineer 6', 'Cloud-native backend systems.', 'Globex Inc'),
  ('pos_01hzzzpos00000000000000010', 'rec_01hzzzrecruiterrole0000000', 'Backend Engineer 7', 'Building resilient distributed services.', 'Initech'),

  ('pos_01hzzzpos00000000000000011', 'rec_01hzzzrecruiterrole0000000', 'Marketing Specialist (Bad Fit) 1', 'Managing digital campaigns and ads.', 'Umbrella Co'),
  ('pos_01hzzzpos00000000000000012', 'rec_01hzzzrecruiterrole0000000', 'Sales Manager (Bad Fit) 2', 'Enterprise sales and client acquisition.', 'Wayne Enterprises'),
  ('pos_01hzzzpos00000000000000013', 'rec_01hzzzrecruiterrole0000000', 'HR Coordinator (Bad Fit) 3', 'Handling HR operations and onboarding.', 'Stark Industries'),
  ('pos_01hzzzpos00000000000000014', 'rec_01hzzzrecruiterrole0000000', 'Graphic Designer (Bad Fit) 4', 'Creating marketing visuals and assets.', 'Umbrella Co'),
  ('pos_01hzzzpos00000000000000015', 'rec_01hzzzrecruiterrole0000000', 'Customer Support Rep (Bad Fit) 5', 'Handling customer inquiries and tickets.', 'Wayne Enterprises'),
  ('pos_01hzzzpos00000000000000016', 'rec_01hzzzrecruiterrole0000000', 'Data Entry Clerk (Bad Fit) 6', 'Manual data processing and entry.', 'Stark Industries'),
  ('pos_01hzzzpos00000000000000017', 'rec_01hzzzrecruiterrole0000000', 'Chef (Bad Fit) 7', 'Food preparation and kitchen operations.', 'Umbrella Co'),
  ('pos_01hzzzpos00000000000000018', 'rec_01hzzzrecruiterrole0000000', 'Teacher (Bad Fit) 8', 'Education and classroom instruction.', 'Wayne Enterprises'),
  ('pos_01hzzzpos00000000000000019', 'rec_01hzzzrecruiterrole0000000', 'Nurse (Bad Fit) 9', 'Clinical care and patient support.', 'Stark Industries'),
  ('pos_01hzzzpos00000000000000020', 'rec_01hzzzrecruiterrole0000000', 'Recruiting Coordinator (Bad Fit) 10', 'Scheduling interviews and candidate management.', 'Umbrella Co')
on conflict (id) do nothing;

-- candidate embedding job
insert into embedding_jobs (id, entity_type, entity_id, status)
values
  ('job_can_01hzzzcand00000000000000000', 'candidate', 'can_01hzzzcand00000000000000000', 'pending')
on conflict (id) do nothing;

-- position embedding jobs
insert into embedding_jobs (id, entity_type, entity_id, status)
values
  ('job_pos_01hzzzpos00000000000000004', 'position', 'pos_01hzzzpos00000000000000004', 'pending'),
  ('job_pos_01hzzzpos00000000000000005', 'position', 'pos_01hzzzpos00000000000000005', 'pending'),
  ('job_pos_01hzzzpos00000000000000006', 'position', 'pos_01hzzzpos00000000000000006', 'pending'),
  ('job_pos_01hzzzpos00000000000000007', 'position', 'pos_01hzzzpos00000000000000007', 'pending'),
  ('job_pos_01hzzzpos00000000000000008', 'position', 'pos_01hzzzpos00000000000000008', 'pending'),
  ('job_pos_01hzzzpos00000000000000009', 'position', 'pos_01hzzzpos00000000000000009', 'pending'),
  ('job_pos_01hzzzpos00000000000000010', 'position', 'pos_01hzzzpos00000000000000010', 'pending'),

  ('job_pos_01hzzzpos00000000000000011', 'position', 'pos_01hzzzpos00000000000000011', 'pending'),
  ('job_pos_01hzzzpos00000000000000012', 'position', 'pos_01hzzzpos00000000000000012', 'pending'),
  ('job_pos_01hzzzpos00000000000000013', 'position', 'pos_01hzzzpos00000000000000013', 'pending'),
  ('job_pos_01hzzzpos00000000000000014', 'position', 'pos_01hzzzpos00000000000000014', 'pending'),
  ('job_pos_01hzzzpos00000000000000015', 'position', 'pos_01hzzzpos00000000000000015', 'pending'),
  ('job_pos_01hzzzpos00000000000000016', 'position', 'pos_01hzzzpos00000000000000016', 'pending'),
  ('job_pos_01hzzzpos00000000000000017', 'position', 'pos_01hzzzpos00000000000000017', 'pending'),
  ('job_pos_01hzzzpos00000000000000018', 'position', 'pos_01hzzzpos00000000000000018', 'pending'),
  ('job_pos_01hzzzpos00000000000000019', 'position', 'pos_01hzzzpos00000000000000019', 'pending'),
  ('job_pos_01hzzzpos00000000000000020', 'position', 'pos_01hzzzpos00000000000000020', 'pending')
on conflict (id) do nothing;
