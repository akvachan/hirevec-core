-- Creates:
--   - 1 candidate user with a strong profile
--   - 3 recruiter users
--   - 5 positions (one per recommendation)
--   - 5 recommendations
--   - 3 positive recruiter reactions (one from each recruiter)
--
-- Safe to run multiple times because it uses INSERT OR IGNORE.

begin transaction;

insert or ignore into users (
    id,
    provider,
    provider_user_id,
    email,
    full_name,
    user_name,
    password_hash,
    updated_at
) values (
    'usr_demo_candidate',
    'email',
    null,
    'alex.chen.demo@example.com',
    'Alex Chen',
    'alexchen',
    '$2a$10$frI8RQRhw9ec7lS8QVd/VeeHHJA.fzyCrHuOXrtiPOzsaXOhE3/T2',
    '2026-07-01T10:00:00Z'
);

insert or ignore into candidates (
    id,
    user_id,
    about,
    last_recommended_at
) values (
    'cand_demo',
    'usr_demo_candidate',
    'Senior Full-Stack Engineer with 8+ years building scalable SaaS platforms. Expert in TypeScript, React, Node.js, Go and AWS. Led teams of up to 10 engineers, shipped products used by millions of users, and enjoys mentoring, system design and developer experience.',
    '2026-07-01T10:00:00Z'
);

insert or ignore into users values (
    'usr_rec_1',
    'email',
    null,
    'sarah@nova.io',
    'Sarah Williams',
    'sarahrecruits',
    '$2a$10$frI8RQRhw9ec7lS8QVd/VeeHHJA.fzyCrHuOXrtiPOzsaXOhE3/T2',
    '2026-07-01T10:00:00Z'
);

insert or ignore into recruiters values (
    'rec_1',
    'usr_rec_1'
);

insert or ignore into users values (
    'usr_rec_2',
    'email',
    null,
    'michael@brightlabs.io',
    'Michael Rodriguez',
    'michaeltalent',
    '$2a$10$frI8RQRhw9ec7lS8QVd/VeeHHJA.fzyCrHuOXrtiPOzsaXOhE3/T2',
    '2026-07-01T10:00:00Z'
);

insert or ignore into recruiters values (
    'rec_2',
    'usr_rec_2'
);

insert or ignore into users values (
    'usr_rec_3',
    'email',
    null,
    'emily@cloudforge.io',
    'Emily Johnson',
    'emilyhires',
    '$2a$10$frI8RQRhw9ec7lS8QVd/VeeHHJA.fzyCrHuOXrtiPOzsaXOhE3/T2',
    '2026-07-01T10:00:00Z'
);

insert or ignore into recruiters values (
    'rec_3',
    'usr_rec_3'
);

insert or ignore into positions values (
    'pos_1',
    'rec_1',
    'Senior Full Stack Engineer',
    'Build customer-facing SaaS applications with React, TypeScript and Node.js.',
    'Nova AI',
    1
);

insert or ignore into positions values (
    'pos_2',
    'rec_2',
    'Staff Backend Engineer',
    'Design distributed systems and APIs powering cloud infrastructure.',
    'Bright Labs',
    1
);

insert or ignore into positions values (
    'pos_3',
    'rec_3',
    'Engineering Manager',
    'Lead a high-performing product engineering team.',
    'CloudForge',
    1
);

insert or ignore into positions values (
    'pos_4',
    'rec_1',
    'Principal Platform Engineer',
    'Drive platform reliability, developer productivity and cloud architecture.',
    'Nova AI',
    1
);

insert or ignore into positions values (
    'pos_5',
    'rec_2',
    'Lead Solutions Architect',
    'Partner with enterprise customers on large-scale cloud adoption.',
    'Bright Labs',
    1
);

insert or ignore into recommendations values (
    'recmd_1',
    'pos_1',
    'cand_demo'
);

insert or ignore into recommendations values (
    'recmd_2',
    'pos_2',
    'cand_demo'
);

insert or ignore into recommendations values (
    'recmd_3',
    'pos_3',
    'cand_demo'
);

insert or ignore into recommendations values (
    'recmd_4',
    'pos_4',
    'cand_demo'
);

insert or ignore into recommendations values (
    'recmd_5',
    'pos_5',
    'cand_demo'
);

insert or ignore into reactions values (
    'recmd_1',
    'recruiter',
    'rec_1',
    'positive',
    '2026-07-01T10:10:00Z'
);

insert or ignore into reactions values (
    'recmd_2',
    'recruiter',
    'rec_2',
    'positive',
    '2026-07-01T10:12:00Z'
);

insert or ignore into reactions values (
    'recmd_3',
    'recruiter',
    'rec_3',
    'positive',
    '2026-07-01T10:15:00Z'
);

commit;
