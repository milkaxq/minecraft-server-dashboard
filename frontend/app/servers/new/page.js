import { NewServer } from "../../../components/new-server";
export default async function Page({ searchParams }) {
  const { plan } = await searchParams;
  return <NewServer initialPlan={typeof plan === "string" ? plan : ""} />;
}
