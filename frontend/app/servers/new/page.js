import { NewServer } from "../../../components/new-server";
export default async function Page({ searchParams }) {
  const { plan, group } = await searchParams;
  return (
    <NewServer
      initialPlan={typeof plan === "string" ? plan : ""}
      initialGroup={typeof group === "string" ? group : "friends"}
    />
  );
}
