-- Seed data: system user, test users, and their accounts.
-- Password for all users: password123
-- bcrypt hash generated with cost 10.

-- System user
INSERT INTO users (id, email, name, password_hash, unique_name, status)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'system@grey.internal',
    'System',
    '$2a$10$B/szfaq.8qQo.9a/Efj4Seyc/kIdzlF4IY4TwnmAiOlQYu927DFbq',
    'system',
    'active'
);

-- System accounts: FX Pool (one per currency, seeded with 1B minor units = $10M equivalent)
INSERT INTO accounts (id, user_id, currency, account_type, balance, status) VALUES
    ('00000000-0000-0000-0001-000000000001', '00000000-0000-0000-0000-000000000001', 'USD', 'fx_pool',  1000000000, 'active'),
    ('00000000-0000-0000-0001-000000000002', '00000000-0000-0000-0000-000000000001', 'EUR', 'fx_pool',  1000000000, 'active'),
    ('00000000-0000-0000-0001-000000000003', '00000000-0000-0000-0000-000000000001', 'GBP', 'fx_pool',  1000000000, 'active');

-- System accounts: Outgoing clearing (one per currency, start at zero)
INSERT INTO accounts (id, user_id, currency, account_type, balance, status) VALUES
    ('00000000-0000-0000-0002-000000000001', '00000000-0000-0000-0000-000000000001', 'USD', 'outgoing', 0, 'active'),
    ('00000000-0000-0000-0002-000000000002', '00000000-0000-0000-0000-000000000001', 'EUR', 'outgoing', 0, 'active'),
    ('00000000-0000-0000-0002-000000000003', '00000000-0000-0000-0000-000000000001', 'GBP', 'outgoing', 0, 'active');

-- Test user: Alice
INSERT INTO users (id, email, name, password_hash, unique_name, status)
VALUES (
    '00000000-0000-0000-0000-000000000002',
    'alice@test.com',
    'Alice',
    '$2a$10$B/szfaq.8qQo.9a/Efj4Seyc/kIdzlF4IY4TwnmAiOlQYu927DFbq',
    'alice',
    'active'
);

-- Test user: Bob
INSERT INTO users (id, email, name, password_hash, unique_name, status)
VALUES (
    '00000000-0000-0000-0000-000000000003',
    'bob@test.com',
    'Bob',
    '$2a$10$B/szfaq.8qQo.9a/Efj4Seyc/kIdzlF4IY4TwnmAiOlQYu927DFbq',
    'bob',
    'active'
);

-- Test user: Charlie
INSERT INTO users (id, email, name, password_hash, unique_name, status)
VALUES (
    '00000000-0000-0000-0000-000000000004',
    'charlie@test.com',
    'Charlie',
    '$2a$10$B/szfaq.8qQo.9a/Efj4Seyc/kIdzlF4IY4TwnmAiOlQYu927DFbq',
    'charlie',
    'active'
);

-- Alice's accounts: USD ($10,000 = 1,000,000 cents), EUR (€5,000 = 500,000 cents)
INSERT INTO accounts (id, user_id, currency, account_type, balance, status) VALUES
    ('00000000-0000-0000-0003-000000000001', '00000000-0000-0000-0000-000000000002', 'USD', 'user', 1000000, 'active'),
    ('00000000-0000-0000-0003-000000000002', '00000000-0000-0000-0000-000000000002', 'EUR', 'user', 500000,  'active');

-- Bob's accounts: USD ($5,000 = 500,000 cents), GBP (£3,000 = 300,000 pence)
INSERT INTO accounts (id, user_id, currency, account_type, balance, status) VALUES
    ('00000000-0000-0000-0003-000000000003', '00000000-0000-0000-0000-000000000003', 'USD', 'user', 500000,  'active'),
    ('00000000-0000-0000-0003-000000000004', '00000000-0000-0000-0000-000000000003', 'GBP', 'user', 300000,  'active');

-- Charlie's accounts: EUR (€8,000 = 800,000 cents)
INSERT INTO accounts (id, user_id, currency, account_type, balance, status) VALUES
    ('00000000-0000-0000-0003-000000000005', '00000000-0000-0000-0000-000000000004', 'EUR', 'user', 800000, 'active');
