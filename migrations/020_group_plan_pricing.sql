-- Separate the default four-player and eight-player recommendations.
-- Keep existing prices/resource limits and order/subscription snapshots intact.
UPDATE plans SET player_recommendation=4, updated_at=now()
WHERE id='10000000-0000-4000-8000-000000000001'
  AND name='Starter' AND currency='USD' AND billing_period='MONTHLY'
  AND price_cents=600 AND ram_mb=2048 AND player_recommendation=10;

UPDATE plans SET player_recommendation=16, updated_at=now()
WHERE id='10000000-0000-4000-8000-000000000002'
  AND name='Survival' AND currency='USD' AND billing_period='MONTHLY'
  AND price_cents=1200 AND ram_mb=4096 AND player_recommendation=30;

INSERT INTO plans
(id,name,description,price_cents,currency,billing_period,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit,player_recommendation)
VALUES ('10000000-0000-4000-8000-000000000004','Game Night','Room for your regular game night.',900,'USD','MONTHLY',3072,15360,150,2,0,8)
ON CONFLICT (id) DO NOTHING;
