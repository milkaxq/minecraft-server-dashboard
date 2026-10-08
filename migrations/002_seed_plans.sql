-- Sample pricing is illustrative; edit in PostgreSQL until the Phase 5 admin API exists.
INSERT INTO plans (id,name,description,price_cents,billing_period,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit,player_recommendation) VALUES
 ('10000000-0000-4000-8000-000000000001','Starter','A small world for friends.',600,'MONTHLY',2048,10240,100,2,0,10),
 ('10000000-0000-4000-8000-000000000002','Survival','Room for a growing community.',1200,'MONTHLY',4096,20480,200,3,1,30),
 ('10000000-0000-4000-8000-000000000003','Pro','Larger worlds and communities.',2400,'MONTHLY',8192,40960,300,5,2,60);
