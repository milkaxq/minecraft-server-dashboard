import { MembersPage } from "../../../../components/members";
export default async function Page({ params }) {
  const { id } = await params;
  return <MembersPage id={id} />;
}
